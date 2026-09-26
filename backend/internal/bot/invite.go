package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
)

// PollInviter executes jobs of type poll_invite (notify.TypePollInvite): it sends
// the support-poll message with voting buttons to one verified owner (docs/02, шаг 2).
type PollInviter struct {
	Messages    Messages
	Initiatives PollingReader
	Houses      HouseReader
	Me          Identity // the open_app button must name this bot
}

// PollingReader reads poll-relevant initiative fields (the initiatives module).
type PollingReader interface {
	Polling(ctx context.Context, id string) (initiatives.Polling, error)
}

// HouseReader reads a house by id (the registry module).
type HouseReader interface {
	House(ctx context.Context, id string) (registry.HouseRef, error)
}

// HandleJob implements notify.JobHandler for poll_invite payloads.
func (p *PollInviter) HandleJob(ctx context.Context, payload json.RawMessage) error {
	var job struct {
		InitiativeID string `json:"initiative_id"`
		MaxUserID    int64  `json:"max_user_id"`
	}
	if err := json.Unmarshal(payload, &job); err != nil {
		return fmt.Errorf("poll invite payload: %w", err)
	}
	if job.InitiativeID == "" || job.MaxUserID <= 0 {
		return fmt.Errorf("poll invite payload: initiative_id %q, max_user_id %d", job.InitiativeID, job.MaxUserID)
	}

	// The job may run later than it was queued (retries, a stopped worker): by then
	// the initiative may be hidden, cancelled or past the poll. Voting buttons must
	// not go out then, and there is nothing to retry.
	initiative, err := p.Initiatives.Polling(ctx, job.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("poll invite initiative: %w", err)
	}
	if initiative.Stage != "poll" {
		return nil
	}
	house, err := p.Houses.House(ctx, initiative.HouseID)
	if err != nil {
		return fmt.Errorf("poll invite house: %w", err)
	}

	text, kb := pollMessage(p.Me, initiative, house, nil)
	msg := maxapi.NewMessage().SetUser(job.MaxUserID).SetText(text).AddKeyboard(kb)
	if _, err := p.Messages.Send(ctx, msg); err != nil {
		return fmt.Errorf("poll invite send: %w", err)
	}

	return nil
}

// pollMessage is the poll message: the invitation and, after a press, the same
// message with the voter's current choice. Once the poll is over the voting
// buttons are gone and only the button of the app stays.
func pollMessage(me Identity, initiative initiatives.Polling, house registry.HouseRef, vote *poll.CastResult) (string, *model.Keyboard) {
	text := fmt.Sprintf("Опрос поддержки: «%s»\n%s\n\n", initiative.Title, house.Address)
	text += "Это опрос, чтобы понять мнение соседей. Он не является голосованием общего собрания и юридической силы не имеет."

	open := initiative.Stage == "poll"
	switch {
	case !open:
		text += "\n\nОпрос завершён."
	case initiative.PollEndsAt != nil:
		text += "\n\nОпрос идёт до " + formatDeadline(*initiative.PollEndsAt, house.Location()) + "."
	}
	if vote != nil {
		text += "\n\nВаш голос: " + voteSummary(*vote) + "."
		if open {
			text += " Изменить можно до конца опроса."
		}
	} else if open {
		text += "\n\nПоддержите идею или выскажите сомнение — от этого зависит, пойдёт ли идея дальше."
	}
	if house.IsDemo {
		text += "\n\n(демо-дом: все данные синтетические)"
	}

	kb := model.NewKeyboard()
	if open {
		kb.AddRow().
			AddButton(voteButton("Поддерживаю", initiative.ID, "for")).
			AddButton(voteButton("Против", initiative.ID, "against"))
		kb.AddRow().
			AddButton(voteButton("Есть вопрос", initiative.ID, "question"))
	}
	kb.AddRow().AddButton(appButton(me, "Открыть приложение", house.InviteSlug))

	return text, kb
}

func voteButton(text, initiativeID, action string) model.Button {
	return model.Button{
		Type:    model.ButtonCallback,
		Text:    text,
		Payload: votePayloadPrefix + ":" + initiativeID + ":" + action,
	}
}

var monthsGenitive = [...]string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

// formatDeadline shows a moment in the house's local time: «3 октября, 18:00».
func formatDeadline(t time.Time, loc *time.Location) string {
	local := t.In(loc)

	return fmt.Sprintf("%d %s, %02d:%02d", local.Day(), monthsGenitive[local.Month()-1], local.Hour(), local.Minute())
}
