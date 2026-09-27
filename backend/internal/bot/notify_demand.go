package bot

// System notification: the demand was delivered to the management company
// (К3). The staff of the organization is notified; in the demo house — the
// initiator alone (docs/04, решение 79).

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"

	"maxhackathon/backend/internal/demand"
)

// DemandAnnouncer reads the facts of a delivered demand (the demand module).
type DemandAnnouncer interface {
	Announce(ctx context.Context, demandID string) (demand.Announce, error)
}

// StaffNotifiees gives the MAX ids of the staff to notify (the access module).
// In the demo house it is the initiator alone (решение 79).
type StaffNotifiees interface {
	StaffRecipients(ctx context.Context, houseID string, initiatorUserID *string) ([]int64, error)
}

// DemandDeliveredNotifier tells the staff (in the demo house — the initiator)
// that the demand was handed to the management company.
type DemandDeliveredNotifier struct {
	Messages Messages
	Announce DemandAnnouncer
	Staff    StaffNotifiees
	Log      *slog.Logger
}

func (n *DemandDeliveredNotifier) HandleJob(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		DemandID string `json:"demand_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("demand delivered payload: %w", err)
	}
	if p.DemandID == "" {
		return fmt.Errorf("demand delivered payload: empty demand_id")
	}

	a, err := n.Announce.Announce(ctx, p.DemandID)
	if err != nil {
		return fmt.Errorf("announce demand %s: %w", p.DemandID, err)
	}

	due := "не указан"
	if !a.UKDueAt.IsZero() {
		due = a.UKDueAt.Format("02.01.2006")
	}

	text := fmt.Sprintf(
		"Поступило требование о проведении общего собрания: «%s»\n%s\n"+
			"Поддержка: %s м² из %s м² (10%% площади дома).\n"+
			"Срок ответа управляющей организации — 45 дней с даты передачи (до %s).",
		a.Title, a.HouseAddress, a.SupportM2, a.ThresholdM2, due)

	ids, err := n.Staff.StaffRecipients(ctx, a.HouseID, a.InitiatorUserID)
	if err != nil {
		return fmt.Errorf("staff recipients: %w", err)
	}
	for _, id := range ids {
		m := maxapi.NewMessage().SetText(text).SetUser(id)
		if _, err := n.Messages.Send(ctx, m); err != nil {
			n.Log.Error("bot: notify staff", "max_user_id", id, "err", err)

			return fmt.Errorf("notify staff %d: %w", id, err)
		}
	}

	return nil
}
