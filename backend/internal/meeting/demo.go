package meeting

// Demo accelerators (docs/plan-do-30-09.md, Г6): without them the jury would wait for
// the voting dates and enter dozens of paper ballots by hand. They work only in the
// demo house (решение 51) and only for the staff running the meeting.

import (
	"context"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
)

var (
	// fillParticipants: the filled ballots bring the participants to 80% of the area.
	fillParticipants = big.NewRat(4, 5)
	// fillFor: «for» on every question up to 85% of the participants. 0.8 × 0.85 =
	// 0.68 ≥ 2/3, so a question under «two thirds of all» passes (≈2 040 м² of 3 000).
	fillFor = big.NewRat(17, 20)
)

// FinishVoting ends the voting now: the decisions can be entered and the result
// fixed. The notice and the start move back as well when they are still ahead, so
// the dates keep their order.
func (s *Service) FinishVoting(ctx context.Context, meetingID, byUserID string) (View, error) {
	l, err := s.loadDemo(ctx, meetingID, byUserID)
	if err != nil {
		return View{}, err
	}
	if l.meeting.Status == StatusCompleted {
		return View{}, ErrAlreadyFinalized
	}

	if _, err := s.pool.Exec(ctx, `
		UPDATE meetings
		SET notice_at = LEAST(notice_at, $2::timestamptz - interval '2 minutes'),
		    voting_starts_at = LEAST(voting_starts_at, $2::timestamptz - interval '1 minute'),
		    voting_ends_at = LEAST(voting_ends_at, $2::timestamptz),
		    updated_at = now()
		WHERE id = $1::uuid AND status = 'preparation'`, l.meeting.ID, s.now()); err != nil {
		return View{}, fmt.Errorf("finish voting: %w", err)
	}

	return s.Get(ctx, l.meeting.ID, byUserID)
}

// FillBallots marks paper ballots as received and counted with plausible decisions
// (fillPlan) until the demo meeting has its quorum and the cameras pass. A second
// call changes nothing.
func (s *Service) FillBallots(ctx context.Context, meetingID, byUserID string) (View, error) {
	l, err := s.loadDemo(ctx, meetingID, byUserID)
	if err != nil {
		return View{}, err
	}
	if l.meeting.Status == StatusCompleted {
		return View{}, ErrAlreadyFinalized
	}
	if !l.meeting.VotingOver(s.now()) {
		return View{}, ErrVotingNotFinished
	}
	owners, err := s.registry.SnapshotOwners(ctx, l.initiative.RegistryUploadID)
	if err != nil {
		return View{}, err
	}
	total := registry.CentiToM2(l.initiative.TotalAreaCenti)

	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// An exclusive lock: a second click waits and then finds nothing to fill.
		if err := lockMeeting(ctx, tx, l.meeting.ID, "FOR UPDATE"); err != nil {
			return err
		}
		ballots, err := s.ballots(ctx, tx, l.meeting.ID)
		if err != nil {
			return err
		}
		counted, err := s.countedBallots(ctx, tx, l.meeting.ID)
		if err != nil {
			return err
		}
		plan := fillPlan(total, fillCandidates(owners, ballots), counted, l.initiative.AgendaItems)
		if len(plan) == 0 {
			return nil
		}

		ids := make([]string, 0, len(plan))
		var ballotIDs, itemIDs, choices []string
		for _, p := range plan {
			ids = append(ids, p.ID)
			for _, item := range l.initiative.AgendaItems {
				ballotIDs = append(ballotIDs, p.ID)
				itemIDs = append(itemIDs, item.ID)
				choices = append(choices, p.Choices[item.ID])
			}
		}
		tag, err := tx.Exec(ctx, `
			UPDATE ballots
			SET status = 'counted', channel = 'paper', received_at = COALESCE(received_at, now()),
			    received_by_user_id = COALESCE(received_by_user_id, $2::uuid), counted_at = now(), updated_at = now()
			WHERE id = ANY($1::uuid[]) AND status IN ('not_voted', 'paper_received')`, ids, byUserID)
		if err != nil {
			return fmt.Errorf("fill ballots: %w", err)
		}
		if int(tag.RowsAffected()) != len(ids) {
			return fmt.Errorf("fill ballots: %d of %d ballots changed", tag.RowsAffected(), len(ids))
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO ballot_decisions (ballot_id, agenda_item_id, choice, recorded_by_user_id)
			SELECT d.ballot, d.item, d.choice, $4::uuid
			FROM unnest($1::uuid[], $2::uuid[], $3::text[]) AS d (ballot, item, choice)
			ON CONFLICT (ballot_id, agenda_item_id) DO UPDATE
			SET choice = EXCLUDED.choice, recorded_by_user_id = EXCLUDED.recorded_by_user_id, updated_at = now()`,
			ballotIDs, itemIDs, choices, byUserID); err != nil {
			return fmt.Errorf("fill decisions: %w", err)
		}

		return nil
	})
	if err != nil {
		return View{}, err
	}

	return s.Get(ctx, l.meeting.ID, byUserID)
}

// loadDemo loads a meeting for a demo accelerator: the staff running it, the demo
// house only.
func (s *Service) loadDemo(ctx context.Context, meetingID, userID string) (loaded, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return loaded{}, err
	}
	if err := s.mustRun(ctx, userID, l); err != nil {
		return loaded{}, err
	}
	if !l.house.IsDemo {
		return loaded{}, ErrNotDemo
	}

	return l, nil
}

// fillBallot is a ballot the demo may fill: not counted yet.
type fillBallot struct {
	ID       string
	WeightM2 *big.Rat
}

// fillCandidates returns the ballots not counted yet in the order of the tracker.
func fillCandidates(owners []registry.SnapshotOwner, ballots []ballot) []fillBallot {
	byOwner := make(map[string]ballot, len(ballots))
	for _, b := range ballots {
		byOwner[b.OwnerID] = b
	}
	result := make([]fillBallot, 0, len(ballots))
	for _, o := range owners {
		b, ok := byOwner[o.ID]
		if ok && (b.Status == BallotNotVoted || b.Status == BallotPaperReceived) {
			result = append(result, fillBallot{ID: b.ID, WeightM2: b.Weight.M2()})
		}
	}

	return result
}

// filled is a ballot the demo fills with its choices by agenda item id.
type filled struct {
	ID      string
	Choices map[string]string
}

// fillPlan picks the candidates in order until the participants (the counted
// ballots and the picked ones) hold 80% of the area, then gives every question
// «for» until it has 85% of the participants and «against» and «abstain» in turn
// after that. Counted ballots keep their decisions and count towards both targets.
func fillPlan(total *big.Rat, candidates []fillBallot, counted []countedBallot, agenda []initiatives.AgendaItem) []filled {
	participants := new(big.Rat)
	for _, b := range counted {
		participants.Add(participants, b.WeightM2)
	}
	target := new(big.Rat).Mul(total, fillParticipants)
	var picked []fillBallot
	for _, c := range candidates {
		if participants.Cmp(target) >= 0 {
			break
		}
		picked = append(picked, c)
		participants.Add(participants, c.WeightM2)
	}

	forTarget := new(big.Rat).Mul(participants, fillFor)
	forSoFar := make(map[string]*big.Rat, len(agenda))
	others := make(map[string]int, len(agenda))
	for _, item := range agenda {
		forSoFar[item.ID] = new(big.Rat)
		for _, b := range counted {
			if b.Choices[item.ID] == ChoiceFor {
				forSoFar[item.ID].Add(forSoFar[item.ID], b.WeightM2)
			}
		}
	}

	plan := make([]filled, 0, len(picked))
	for _, c := range picked {
		f := filled{ID: c.ID, Choices: make(map[string]string, len(agenda))}
		for _, item := range agenda {
			switch {
			case forSoFar[item.ID].Cmp(forTarget) < 0:
				f.Choices[item.ID] = ChoiceFor
				forSoFar[item.ID].Add(forSoFar[item.ID], c.WeightM2)
			case others[item.ID]%2 == 0:
				f.Choices[item.ID] = ChoiceAgainst
				others[item.ID]++
			default:
				f.Choices[item.ID] = ChoiceAbstain
				others[item.ID]++
			}
		}
		plan = append(plan, f)
	}

	return plan
}
