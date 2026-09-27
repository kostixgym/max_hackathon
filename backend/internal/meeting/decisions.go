package meeting

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/initiatives"
)

// Decision is the choice of a ballot on one agenda question.
type Decision struct {
	AgendaItemID string
	Choice       string
}

// BallotDecisions is a counted paper ballot.
type BallotDecisions struct {
	BallotID  string
	Status    string
	Decisions []Decision
}

// RecordDecisions enters the decisions of a paper ballot after the end of the voting
// (решение 26), one per agenda question, and the ballot becomes counted. Until the
// result is fixed a mistake can be corrected by entering the ballot again.
func (s *Service) RecordDecisions(ctx context.Context, ballotID, byUserID string, decisions []Decision) (BallotDecisions, error) {
	if !validID(ballotID) {
		return BallotDecisions{}, ErrBallotNotFound
	}
	var meetingID string
	err := s.pool.QueryRow(ctx, `SELECT meeting_id::text FROM ballots WHERE id = $1::uuid`, ballotID).Scan(&meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return BallotDecisions{}, ErrBallotNotFound
	}
	if err != nil {
		return BallotDecisions{}, fmt.Errorf("ballot meeting: %w", err)
	}
	l, err := s.load(ctx, meetingID)
	if errors.Is(err, ErrNotFound) {
		return BallotDecisions{}, ErrBallotNotFound
	}
	if err != nil {
		return BallotDecisions{}, err
	}
	if err := s.mustRun(ctx, byUserID, l); err != nil {
		return BallotDecisions{}, err
	}
	if l.meeting.Status == StatusCompleted {
		return BallotDecisions{}, ErrAlreadyFinalized
	}
	if !l.meeting.VotingOver(s.now()) {
		return BallotDecisions{}, ErrVotingNotFinished
	}
	ordered, err := orderDecisions(l.initiative.AgendaItems, decisions)
	if err != nil {
		return BallotDecisions{}, err
	}
	itemIDs := make([]string, len(ordered))
	choices := make([]string, len(ordered))
	for i, d := range ordered {
		itemIDs[i], choices[i] = d.AgendaItemID, d.Choice
	}

	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// A shared lock: ballots are entered side by side, the fixing waits for them.
		if err := lockMeeting(ctx, tx, l.meeting.ID, "FOR SHARE"); err != nil {
			return err
		}
		var status string
		if err := tx.QueryRow(ctx, `SELECT status FROM ballots WHERE id = $1::uuid FOR UPDATE`,
			ballotID).Scan(&status); err != nil {
			return fmt.Errorf("lock ballot: %w", err)
		}
		if status != BallotPaperReceived && status != BallotCounted {
			return fmt.Errorf("%w: %s", ErrBallotNotReceived, status)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO ballot_decisions (ballot_id, agenda_item_id, choice, recorded_by_user_id)
			SELECT $1::uuid, d.item, d.choice, $4::uuid
			FROM unnest($2::uuid[], $3::text[]) AS d (item, choice)
			ON CONFLICT (ballot_id, agenda_item_id) DO UPDATE
			SET choice = EXCLUDED.choice, recorded_by_user_id = EXCLUDED.recorded_by_user_id, updated_at = now()`,
			ballotID, itemIDs, choices, byUserID); err != nil {
			return fmt.Errorf("record decisions: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE ballots SET status = 'counted', counted_at = now(), updated_at = now()
			WHERE id = $1::uuid`, ballotID); err != nil {
			return fmt.Errorf("count ballot: %w", err)
		}

		return nil
	})
	if err != nil {
		return BallotDecisions{}, err
	}

	return BallotDecisions{BallotID: ballotID, Status: BallotCounted, Decisions: ordered}, nil
}

// orderDecisions checks that the decisions cover every agenda question exactly once
// with a known choice, and returns them in the agenda order.
func orderDecisions(agenda []initiatives.AgendaItem, decisions []Decision) ([]Decision, error) {
	byItem := make(map[string]string, len(decisions))
	for _, d := range decisions {
		switch d.Choice {
		case ChoiceFor, ChoiceAgainst, ChoiceAbstain:
		default:
			return nil, fmt.Errorf("%w: choice %q", ErrInvalidDecisions, d.Choice)
		}
		if _, twice := byItem[d.AgendaItemID]; twice {
			return nil, fmt.Errorf("%w: question %s twice", ErrInvalidDecisions, d.AgendaItemID)
		}
		byItem[d.AgendaItemID] = d.Choice
	}
	if len(byItem) != len(agenda) {
		return nil, fmt.Errorf("%w: %d decisions for %d questions", ErrInvalidDecisions, len(byItem), len(agenda))
	}

	ordered := make([]Decision, 0, len(agenda))
	for _, item := range agenda {
		choice, ok := byItem[item.ID]
		if !ok {
			return nil, fmt.Errorf("%w: no decision on question %d", ErrInvalidDecisions, item.Position)
		}
		ordered = append(ordered, Decision{AgendaItemID: item.ID, Choice: choice})
	}

	return ordered, nil
}
