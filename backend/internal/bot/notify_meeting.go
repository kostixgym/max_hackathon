package bot

// Meeting notifications (К3): a meeting is created — the verified owners of the
// house learn about it; the result is fixed — everyone learns the outcome
// (docs/plan-do-30-09.md, К3). These are important events: quiet hours do not
// delay them (docs/04, решение 29).

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/meeting"
	"maxhackathon/backend/internal/registry"
)

// MeetingBotView reads the meeting card for system notifications (the meeting
// module): no viewer checks, choices never appear.
type MeetingBotView interface {
	BotView(ctx context.Context, meetingID string) (meeting.View, error)
}

// OwnerNotifiees gives the verified owners of the house with their MAX ids
// (the access module).
type OwnerNotifiees interface {
	VerifiedOwnerRecipients(ctx context.Context, houseID string) ([]access.Recipient, error)
}

// notifyOwners delivers a text to every verified owner of the house. The
// initiator is among them: he is a confirmed owner himself (docs/01).
func notifyOwners(ctx context.Context, n OwnerNotifiees, msg Messages,
	log *slog.Logger, houseID, text string) error {
	recipients, err := n.VerifiedOwnerRecipients(ctx, houseID)
	if err != nil {
		return fmt.Errorf("owner recipients: %w", err)
	}

	for _, r := range recipients {
		m := maxapi.NewMessage().SetText(text).SetUser(r.MaxUserID)
		if _, err := msg.Send(ctx, m); err != nil {
			log.Error("bot: notify owner", "max_user_id", r.MaxUserID, "err", err)

			return fmt.Errorf("notify owner %d: %w", r.MaxUserID, err)
		}
	}

	return nil
}

// MeetingCreatedNotifier tells the verified owners that a meeting has been
// scheduled for their initiative.
type MeetingCreatedNotifier struct {
	Messages Messages
	View     MeetingBotView
	Owners   OwnerNotifiees
	Log      *slog.Logger
}

func (n *MeetingCreatedNotifier) HandleJob(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		MeetingID string `json:"meeting_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("meeting created payload: %w", err)
	}
	if p.MeetingID == "" {
		return fmt.Errorf("meeting created payload: empty meeting_id")
	}

	v, err := n.View.BotView(ctx, p.MeetingID)
	if err != nil {
		return fmt.Errorf("meeting view: %w", err)
	}

	text := fmt.Sprintf(
		"Назначено собрание по инициативе «%s» (дом: %s).\nГолосование: %s.\nПовестка: %d вопрос(ов). "+
			"Отметь свой бюллетень в приложении.",
		v.Title, v.House.Address, humanWindow(v.VotingStartsAt, v.VotingEndsAt), len(v.Agenda))
	if v.House.IsDemo {
		text += "\n(демо-дом: все данные синтетические)"
	}

	return notifyOwners(ctx, n.Owners, n.Messages, n.Log, v.House.ID, text)
}

// MeetingFinalizedNotifier tells the initiator and the verified owners the
// outcome of the meeting, question by question.
type MeetingFinalizedNotifier struct {
	Messages Messages
	View     MeetingBotView
	Protocol ProtocolReader
	Owners   OwnerNotifiees
	Log      *slog.Logger
}

// ProtocolReader gives the fixed result of the meeting (the meeting module).
type ProtocolReader interface {
	ProtocolData(ctx context.Context, meetingID string) (meeting.Protocol, error)
}

func (n *MeetingFinalizedNotifier) HandleJob(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		MeetingID string `json:"meeting_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("meeting finalized payload: %w", err)
	}
	if p.MeetingID == "" {
		return fmt.Errorf("meeting finalized payload: empty meeting_id")
	}

	v, err := n.View.BotView(ctx, p.MeetingID)
	if err != nil {
		return fmt.Errorf("meeting view: %w", err)
	}
	protocol, err := n.Protocol.ProtocolData(ctx, p.MeetingID)
	if err != nil {
		return fmt.Errorf("protocol data: %w", err)
	}

	text := fmt.Sprintf(
		"Итог собрания по «%s» (дом: %s): %s.\n\nРешения по вопросам:",
		v.Title, v.House.Address, outcomeText(protocol.Outcome))
	for _, item := range protocol.Items {
		verdict := "не принято"
		if item.Accepted {
			verdict = "ПРИНЯТО"
		}
		text += fmt.Sprintf("\n• %s — %s (%s м² «за», %s «против», %s воздержались)",
			strings.TrimSpace(item.Text), verdict,
			registry.FormatM2(item.ForM2), registry.FormatM2(item.AgainstM2), registry.FormatM2(item.AbstainM2))
	}
	text += "\n\nПротокол сформирован и доступен в приложении."

	return notifyOwners(ctx, n.Owners, n.Messages, n.Log, v.House.ID, text)
}

// outcomeText names the meeting outcome in words.
func outcomeText(outcome string) string {
	if outcome == "no_quorum" {
		return "кворума не было, решения не принимались"
	}

	return "собрание состоялось, решения приняты"
}

// humanWindow formats the voting window for a person.
func humanWindow(from, to time.Time) string {
	const layout = "02.01 15:04"
	if from.IsZero() || to.IsZero() {
		return "сроки уточняются"
	}

	return from.Format(layout) + " — " + to.Format(layout)
}
