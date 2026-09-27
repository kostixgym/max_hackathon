package meeting

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ReceivedBallot is a paper ballot marked as handed in.
type ReceivedBallot struct {
	BallotID   string
	Status     string
	ReceivedAt time.Time
}

// ReceiveBallot marks a paper ballot of the tracker as handed in. During the voting
// only the fact is recorded, the choices are entered after its end (решение 26). A
// ballot handed in after the end does not count, so it cannot be received.
func (s *Service) ReceiveBallot(ctx context.Context, meetingID, ballotID, byUserID string) (ReceivedBallot, error) {
	if !validID(ballotID) {
		return ReceivedBallot{}, ErrBallotNotFound
	}
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return ReceivedBallot{}, err
	}
	if err := s.mustRun(ctx, byUserID, l); err != nil {
		return ReceivedBallot{}, err
	}
	if l.meeting.VotingOver(s.now()) {
		return ReceivedBallot{}, ErrVotingFinished
	}

	r := ReceivedBallot{BallotID: ballotID, Status: BallotPaperReceived}
	err = s.pool.QueryRow(ctx, `
		UPDATE ballots
		SET status = 'paper_received', channel = 'paper', received_at = now(),
		    received_by_user_id = $3::uuid, updated_at = now()
		WHERE id = $1::uuid AND meeting_id = $2::uuid AND status = 'not_voted'
		RETURNING received_at`, ballotID, l.meeting.ID, byUserID).Scan(&r.ReceivedAt)
	if err == nil {
		return r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ReceivedBallot{}, fmt.Errorf("receive ballot: %w", err)
	}

	// Nothing updated: either the ballot is not of this meeting or it is already in hand.
	var status string
	err = s.pool.QueryRow(ctx, `SELECT status FROM ballots WHERE id = $1::uuid AND meeting_id = $2::uuid`,
		ballotID, l.meeting.ID).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ReceivedBallot{}, ErrBallotNotFound
	case err != nil:
		return ReceivedBallot{}, fmt.Errorf("ballot status: %w", err)
	default:
		return ReceivedBallot{}, fmt.Errorf("%w: %s", ErrAlreadyReceived, status)
	}
}
