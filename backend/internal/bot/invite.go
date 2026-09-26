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
	Progress    PollProgress // optional: the support so far in the message
	Me          Identity     // the open_app button must name this bot
	Now         func() time.Time
}

// PollingReader reads poll-relevant initiative fields (the initiatives module).
type PollingReader interface {
	Polling(ctx context.Context, id string) (initiatives.Polling, error)
}

// HouseReader reads a house by id (the registry module).
type HouseReader interface {
	House(ctx context.Context, id string) (registry.HouseRef, error)
}

// PollProgress reads the support of the poll in м² (the poll module).
type PollProgress interface {
	Progress(ctx context.Context, initiativeID string) (poll.Progress, error)
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

	// The job may run later than it was queued (retries, a stopped worker, the quiet
	// hours): by then the initiative may be hidden, cancelled or past the poll. Voting
	// buttons must not go out then, and there is nothing to retry.
	initiative, err := p.Initiatives.Polling(ctx, job.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("poll invite initiative: %w", err)
	}
	now := clock(p.Now)
	if !initiative.Open(now) {
		return nil
	}
	house, err := p.Houses.House(ctx, initiative.HouseID)
	if err != nil {
		return fmt.Errorf("poll invite house: %w", err)
	}

	view := pollView{initiative: initiative, house: house, now: now}
	if p.Progress != nil {
		// The support line is a courtesy: without it the invitation still goes out.
		if progress, err := p.Progress.Progress(ctx, initiative.ID); err == nil {
			view.progress = &progress
		}
	}
	text, kb := view.render(p.Me)
	msg := maxapi.NewMessage().SetUser(job.MaxUserID).SetText(text).SetFormat(model.FormatHTML).AddKeyboard(kb)
	if _, err := p.Messages.Send(ctx, msg); err != nil {
		return fmt.Errorf("poll invite send: %w", err)
	}

	return nil
}
