// Package maxbot receives updates from the MAX Bot API (an inbound adapter).
//
// Long polling instead of a webhook: the bot needs no public address, so a local
// `docker compose up` works without tunnels. One token has exactly one consumer of
// updates: never run two pollers with the same token (docs/05, «Риски»).
package maxbot

import (
	"context"
	"errors"
	"log/slog"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"
)

// Updates is the part of the MAX client the poller needs.
type Updates interface {
	GetUpdates(ctx context.Context, marker int64) ([]model.Update, int64, error)
}

// Handler processes one update. It must not block for long: updates are handled
// one by one to keep the order of actions of every user.
type Handler interface {
	Handle(ctx context.Context, u model.Update)
}

// Poller runs the long polling loop.
type Poller struct {
	Updates Updates
	Handler Handler
	Log     *slog.Logger

	// Backoff after a failed request: grows from MinBackoff to MaxBackoff.
	MinBackoff time.Duration
	MaxBackoff time.Duration

	// BotID and Markers persist the polling position in the database: a restart
	// continues after the last fully processed batch instead of re-reading the
	// history. The marker is saved after the whole batch (at-least-once); vote
	// handlers are idempotent upserts, a repeated greeting is acceptable.
	BotID   int64
	Markers Markers
}

// Markers loads and saves the long-polling position (the notify module).
type Markers interface {
	LoadMarker(ctx context.Context, botUserID int64) (int64, error)
	SaveMarker(ctx context.Context, botUserID int64, marker int64) error
}

// Run polls until ctx is cancelled. Errors of the MAX API are logged and retried:
// the HTTP API of the mini-app keeps working even if MAX is unreachable.
func (p *Poller) Run(ctx context.Context) {
	minB, maxB := p.MinBackoff, p.MaxBackoff
	if minB <= 0 {
		minB = time.Second
	}
	if maxB < minB {
		maxB = 30 * time.Second
	}

	var marker int64
	if p.Markers != nil && p.BotID != 0 {
		var err error
		if marker, err = p.Markers.LoadMarker(ctx, p.BotID); err != nil {
			p.Log.Warn("max: load marker, starting from 0", "err", err)
		}
	}

	backoff := minB
	for ctx.Err() == nil {
		updates, next, err := p.Updates.GetUpdates(ctx, marker)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			// Long polling timeout without updates is the normal case.
			if _, ok := errors.AsType[*maxapi.TimeoutError](err); ok {
				continue
			}
			p.Log.Warn("max: get updates failed, retrying", "err", err, "in", backoff)
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, maxB)

			continue
		}

		backoff = minB
		marker = next
		for _, u := range updates {
			p.handle(ctx, u)
		}
		// The batch is fully handled: it is safe to persist the position. A crash
		// before this point replays the batch, it does not lose its tail.
		if p.Markers != nil && p.BotID != 0 {
			if err := p.Markers.SaveMarker(ctx, p.BotID, next); err != nil {
				p.Log.Error("max: save marker", "err", err)
			}
		}
	}
}

// handle isolates a panic in one update so that it does not stop the bot.
func (p *Poller) handle(ctx context.Context, u model.Update) {
	defer func() {
		if v := recover(); v != nil {
			p.Log.Error("max: panic in update handler", "panic", v, "type", u.UpdateType)
		}
	}()
	p.Handler.Handle(ctx, u)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
