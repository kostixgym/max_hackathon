package bot

import (
	"context"
	"encoding/json"
	"fmt"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
)

// PollInviter executes jobs of type poll_invite (notify.TypePollInvite): it sends
// the support-poll message with voting buttons to one verified owner (docs/02, шаг 2).
type PollInviter struct {
	Messages    Messages
	Initiatives PollingReader
	Houses      HouseReader
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

	initiative, err := p.Initiatives.Polling(ctx, job.InitiativeID)
	if err != nil {
		return fmt.Errorf("poll invite initiative: %w", err)
	}
	house, err := p.Houses.House(ctx, initiative.HouseID)
	if err != nil {
		return fmt.Errorf("poll invite house: %w", err)
	}

	msg := maxapi.NewMessage().
		SetUser(job.MaxUserID).
		SetText(pollInviteText(initiative, house)).
		AddKeyboard(pollKeyboard(initiative.ID))
	if _, err := p.Messages.Send(ctx, msg); err != nil {
		return fmt.Errorf("poll invite send: %w", err)
	}

	return nil
}

func pollInviteText(initiative initiatives.Polling, house registry.HouseRef) string {
	text := fmt.Sprintf("Опрос поддержки: «%s»\n%s\n\n", initiative.Title, house.Address)
	text += "Это опрос, чтобы понять мнение соседей. Он не является голосованием общего собрания и юридической силы не имеет.\n\n"
	text += "Поддержите идею или выскажите сомнение — от этого зависит, пойдёт ли идея дальше."
	if house.IsDemo {
		text += "\n\n(демо-дом: все данные синтетические)"
	}

	return text
}

func pollKeyboard(initiativeID string) *model.Keyboard {
	kb := model.NewKeyboard()
	kb.AddRow().
		AddButton(voteButton("Поддерживаю", initiativeID, "for")).
		AddButton(voteButton("Против", initiativeID, "against"))
	kb.AddRow().
		AddButton(voteButton("Есть вопрос", initiativeID, "question"))

	return kb
}

func voteButton(text, initiativeID, action string) model.Button {
	return model.Button{
		Type:    model.ButtonCallback,
		Text:    text,
		Payload: votePayloadPrefix + ":" + initiativeID + ":" + action,
	}
}
