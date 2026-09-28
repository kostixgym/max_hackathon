package initiatives

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Allowed stage transitions of path A (docs/04, «Стадии»): draft → poll is made
// by StartPoll (it sets poll_ends_at together with the stage), the cancel is a
// separate operation with a mandatory reason (решение 39) and does not go here.
var allowedTransitions = map[string][]string{
	StageDraft:   {StagePoll},
	StagePoll:    {StageDemand, StageMeeting}, // путь A и путь B
	StageDemand:  {StageMeeting},
	StageMeeting: {StageCompleted},
}

// SetStageTx moves the initiative from one stage to another inside the caller's
// transaction (docs/plan-do-30-09.md, «Интерфейсы Go между людьми»). Demand and
// meeting creation call it next to their own rows, so the stage can never drift
// from the data: either both happen or neither does.
//
// from is the stage the initiative must be on right now: the optimistic check
// makes two concurrent transitions (say, a demand and a self-organized meeting)
// impossible — the loser gets ErrWrongStage. path is set when the transition
// picks the way (A on the demand, B on a self-organized meeting).
func (s *Service) SetStageTx(ctx context.Context, tx pgx.Tx, id, from, to string, path *string) error {
	if !validID(id) {
		return fmt.Errorf("%w: unknown initiative", ErrWrongStage)
	}
	if !transitionAllowed(from, to) {
		return fmt.Errorf("%w: %s → %s", ErrWrongStage, from, to)
	}

	var pathValue any
	if path != nil {
		pathValue = *path
	}

	tag, err := tx.Exec(ctx, `
		UPDATE initiatives
		SET stage = $3, path = COALESCE($4, path), updated_at = now()
		WHERE id = $1::uuid AND stage = $2 AND hidden_at IS NULL`,
		id, from, to, pathValue)
	if err != nil {
		return fmt.Errorf("set stage %s → %s: %w", from, to, err)
	}

	switch tag.RowsAffected() {
	case 1:
		return nil
	case 0:
		// Hidden or missing: for the caller it is the same as a wrong stage.
		return fmt.Errorf("%w: %s", ErrWrongStage, from)
	default:
		return errors.Join(ErrWrongStage, fmt.Errorf("set stage: %d rows affected", tag.RowsAffected()))
	}
}

func transitionAllowed(from, to string) bool {
	for _, next := range allowedTransitions[from] {
		if next == to {
			return true
		}
	}

	return false
}

// SetStage is the pool version of SetStageTx for callers without their own
// transaction (the bot worker, tests).
func (s *Service) SetStage(ctx context.Context, id, from, to string, path *string) error {
	return s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return s.SetStageTx(ctx, tx, id, from, to, path)
	})
}

// SelectPathB lets the initiator choose self-organized OСС after the support poll
// closes. Persisting the choice unlocks the owner directory for officer selection.
func (s *Service) SelectPathB(ctx context.Context, id, userID string) error {
	in, err := s.Details(ctx, id)
	if err != nil {
		return err
	}
	if in.InitiatorUserID == nil || *in.InitiatorUserID != userID {
		return ErrNotInitiator
	}
	if in.Stage != StagePoll {
		return ErrWrongStage
	}
	if in.PollEndsAt == nil || time.Now().Before(*in.PollEndsAt) {
		return ErrPollStillOpen
	}
	return s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE initiatives SET path = 'B', updated_at = now()
			WHERE id = $1::uuid AND initiator_user_id = $2::uuid AND stage = 'poll' AND path IS NULL AND hidden_at IS NULL`, id, userID)
		if err != nil {
			return fmt.Errorf("select path B: %w", err)
		}
		if tag.RowsAffected() == 1 {
			return nil
		}
		var path *string
		if err := tx.QueryRow(ctx, `SELECT path FROM initiatives WHERE id = $1::uuid`, id).Scan(&path); err != nil {
			return fmt.Errorf("read selected path: %w", err)
		}
		if path != nil && *path == "B" {
			return nil
		}
		return ErrPathAlreadyChosen
	})
}

// Stage returns the current stage of the initiative (0 calls, for the bot).
func (s *Service) Stage(ctx context.Context, id string) (string, error) {
	in, err := s.Get(ctx, id)

	return in.Stage, err
}
