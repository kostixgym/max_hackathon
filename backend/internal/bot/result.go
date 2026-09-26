package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
)

// PollResult executes jobs of type poll_finished (notify.TypePollFinished): when the
// term of the poll ends, the initiator gets the result and the next step (решения 18,
// 77). The voters see the result in the app.
type PollResult struct {
	Messages    Messages
	Initiatives InitiativeReader
	Houses      HouseReader
	Progress    PollProgress
	Accounts    Accounts
	Me          Identity
	Now         func() time.Time
}

// resultSlack: the job runs at the term by the clock of the database; a slightly
// different clock of the app must not make it skip the result.
const resultSlack = time.Minute

// HandleJob implements notify.JobHandler.
func (r *PollResult) HandleJob(ctx context.Context, payload json.RawMessage) error {
	var job struct {
		InitiativeID string `json:"initiative_id"`
	}
	if err := json.Unmarshal(payload, &job); err != nil || job.InitiativeID == "" {
		return fmt.Errorf("poll result payload: %s", payload)
	}

	initiative, err := r.Initiatives.Get(ctx, job.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("poll result initiative: %w", err)
	}
	// The initiator has moved on or cancelled the initiative: the result is not news.
	if initiative.Stage != initiatives.StagePoll || initiative.InitiatorUserID == nil {
		return nil
	}
	// The term was moved later: the job of the new term will report.
	if initiative.PollEndsAt == nil || initiative.PollEndsAt.Sub(clock(r.Now)) > resultSlack {
		return nil
	}

	to, err := r.Accounts.MaxUserID(ctx, *initiative.InitiatorUserID)
	if errors.Is(err, access.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("poll result initiator: %w", err)
	}
	house, err := r.Houses.House(ctx, initiative.HouseID)
	if err != nil {
		return fmt.Errorf("poll result house: %w", err)
	}
	progress, err := r.Progress.Progress(ctx, initiative.ID)
	if err != nil {
		return fmt.Errorf("poll result progress: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<b>Опрос завершён: «%s»</b>\n%s\n\n", esc(initiative.Title), esc(house.Address))
	fmt.Fprintf(&b, "За — <b>%s м²</b> (%s%% площади дома), против — %s м².\n",
		registry.FormatM2(progress.ForM2()), percentOf(progress.ForM2(), progress.TotalM2()),
		registry.FormatM2(progress.AgainstM2()))
	fmt.Fprintf(&b, "Проголосовало собственников: %d.\n\n", progress.VotesFor+progress.VotesAgainst)
	demand := registry.FormatM2(progress.Thresholds().Demand)
	if progress.DemandReached() {
		fmt.Fprintf(&b, "<mark>Поддержки достаточно, чтобы потребовать от УК провести общее собрание</mark> "+
			"(ст. 45 ч. 6 ЖК): нужно не меньше 10%% площади дома — %s м².", demand)
	} else {
		fmt.Fprintf(&b, "Для требования в УК поддержки не хватило: нужно не меньше 10%% площади дома — %s м². "+
			"Собрание можно провести и самим, без УК.", demand)
	}
	b.WriteString("\n\nСледующий шаг выбирается в приложении.")
	if house.IsDemo {
		b.WriteString("\n\n<i>Демо-дом: все данные синтетические.</i>")
	}

	msg := maxapi.NewMessage().SetUser(to).SetText(b.String()).SetFormat(model.FormatHTML).
		AddKeyboard(openAppButton(r.Me, "Открыть приложение", house.InviteSlug))
	if _, err := r.Messages.Send(ctx, msg); err != nil {
		return fmt.Errorf("poll result send: %w", err)
	}

	return nil
}
