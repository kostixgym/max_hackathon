package notify

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LoadMarker returns the saved long-polling marker of the bot, 0 for a new bot.
// The poller calls it on start, so a restart continues from the last processed
// update instead of re-reading the whole history.
func (s *Store) LoadMarker(ctx context.Context, botUserID int64) (int64, error) {
	var marker int64
	err := s.pool.QueryRow(ctx, `SELECT marker FROM bot_markers WHERE bot_user_id = $1`, botUserID).Scan(&marker)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load bot marker: %w", err)
	}

	return marker, nil
}

// SaveMarker persists the marker after a batch of updates has been handled.
// Handlers are idempotent (a vote is an upsert, a message may be re-sent), so the
// marker is saved after the whole batch: on a crash the batch is re-processed
// (at-least-once) instead of losing its tail (at-most-once).
func (s *Store) SaveMarker(ctx context.Context, botUserID int64, marker int64) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO bot_markers (bot_user_id, marker) VALUES ($1, $2)
		ON CONFLICT (bot_user_id) DO UPDATE SET marker = EXCLUDED.marker, updated_at = now()`,
		botUserID, marker)
	if err != nil {
		return fmt.Errorf("save bot marker: %w", err)
	}

	return nil
}
