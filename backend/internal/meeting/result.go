package meeting

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
)

// ItemResult is the count of one agenda question.
type ItemResult struct {
	AgendaItemID string
	Position     int
	Text         string
	MajorityRule string
	ForM2        *big.Rat
	AgainstM2    *big.Rat
	AbstainM2    *big.Rat
	Accepted     bool
}

// Result is the count of a meeting: who took part, whether there is a quorum and how
// every question is decided.
type Result struct {
	ParticipantsM2 *big.Rat
	TotalM2        *big.Rat
	QuorumReached  bool
	Items          []ItemResult
}

// countedBallot is a counted paper ballot with its choices by agenda item id.
type countedBallot struct {
	ID       string
	WeightM2 *big.Rat
	Choices  map[string]string
}

// tally counts the official result by the Housing Code: the participants and votes
// are the GIS aggregates plus the counted paper ballots. The quorum is more than
// half of all votes (ст. 45 ч. 3 ЖК), and a question is accepted by the majority of
// its decision type (rules.Passed), only with the quorum. All sums are exact.
func tally(total *big.Rat, ballots []countedBallot, gis GISResults, agenda []initiatives.AgendaItem) (Result, error) {
	if gis.OnlineParticipantsM2 == nil {
		gis.OnlineParticipantsM2 = new(big.Rat)
	}
	orderedGIS, err := orderGISResults(agenda, gis.Entries, gis.OnlineParticipantsM2, total)
	if err != nil {
		return Result{}, err
	}
	byItem := make(map[string]GISResultEntry, len(orderedGIS))
	for _, entry := range orderedGIS {
		byItem[entry.AgendaItemID] = entry
	}

	r := Result{
		ParticipantsM2: new(big.Rat).Set(gis.OnlineParticipantsM2),
		TotalM2:        total,
		Items:          make([]ItemResult, 0, len(agenda)),
	}
	for _, b := range ballots {
		r.ParticipantsM2.Add(r.ParticipantsM2, b.WeightM2)
	}
	if r.ParticipantsM2.Cmp(total) > 0 {
		return Result{}, fmt.Errorf("%w: all participants exceed the registry area", ErrInvalidGISResults)
	}
	r.QuorumReached = rules.QuorumReached(total, r.ParticipantsM2)

	for _, item := range agenda {
		rule, err := rules.ParseRule(item.MajorityRule)
		if err != nil {
			return Result{}, fmt.Errorf("agenda item %d: %w", item.Position, err)
		}
		ir := ItemResult{
			AgendaItemID: item.ID, Position: item.Position, Text: item.Text, MajorityRule: item.MajorityRule,
			ForM2: new(big.Rat), AgainstM2: new(big.Rat), AbstainM2: new(big.Rat),
		}
		if entry, ok := byItem[item.ID]; ok {
			ir.ForM2.Set(entry.ForM2)
			ir.AgainstM2.Set(entry.AgainstM2)
			ir.AbstainM2.Set(entry.AbstainM2)
		}
		for _, b := range ballots {
			switch b.Choices[item.ID] {
			case ChoiceFor:
				ir.ForM2.Add(ir.ForM2, b.WeightM2)
			case ChoiceAgainst:
				ir.AgainstM2.Add(ir.AgainstM2, b.WeightM2)
			case ChoiceAbstain:
				ir.AbstainM2.Add(ir.AbstainM2, b.WeightM2)
			}
		}
		passed, err := rules.Passed(rule, total, r.ParticipantsM2, ir.ForM2)
		if err != nil {
			return Result{}, err
		}
		ir.Accepted = r.QuorumReached && passed
		r.Items = append(r.Items, ir)
	}

	return r, nil
}

func (s *Service) countedBallots(ctx context.Context, q querier, meetingID string) ([]countedBallot, error) {
	rows, err := q.Query(ctx, `
		SELECT b.id::text, b.weight_num, b.weight_den, d.agenda_item_id::text, d.choice
		FROM ballots b
		LEFT JOIN ballot_decisions d ON d.ballot_id = b.id
		WHERE b.meeting_id = $1::uuid AND b.status = 'counted'
		ORDER BY b.id`, meetingID)
	if err != nil {
		return nil, fmt.Errorf("counted ballots: %w", err)
	}
	defer rows.Close()

	result := make([]countedBallot, 0)
	index := map[string]int{}
	for rows.Next() {
		var id string
		var w registry.Weight
		var itemID, choice *string
		if err := rows.Scan(&id, &w.Num, &w.Den, &itemID, &choice); err != nil {
			return nil, fmt.Errorf("scan counted ballot: %w", err)
		}
		i, ok := index[id]
		if !ok {
			i = len(result)
			index[id] = i
			result = append(result, countedBallot{ID: id, WeightM2: w.M2(), Choices: map[string]string{}})
		}
		if itemID != nil && choice != nil {
			result[i].Choices[*itemID] = *choice
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("counted ballots: %w", err)
	}

	return result, nil
}

// Preview counts the result before it is fixed: the administrator checks it and
// fixes it. Decisions are entered only after the voting, so before its end the
// preview has no votes (решение 26).
func (s *Service) Preview(ctx context.Context, meetingID, viewerID string) (Result, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return Result{}, err
	}
	if err := s.mustRun(ctx, viewerID, l); err != nil {
		return Result{}, err
	}
	counted, err := s.countedBallots(ctx, s.pool, l.meeting.ID)
	if err != nil {
		return Result{}, err
	}
	gis, err := s.gisResults(ctx, s.pool, l.meeting.ID)
	if err != nil {
		return Result{}, err
	}

	return tally(registry.CentiToM2(l.initiative.TotalAreaCenti), counted, gis, l.initiative.AgendaItems)
}

// Final is the fixed result of a meeting.
type Final struct {
	MeetingID   string
	Outcome     string
	FinalizedAt time.Time
	Result      Result
}

// Finalize fixes the result once (решение 61). In one transaction: a row per question
// in meeting_results, the participants and the outcome of the meeting, the initiative
// completes, and the bot announces the result. The rows never change afterwards: the
// protocol is built from them.
func (s *Service) Finalize(ctx context.Context, meetingID, byUserID string) (Final, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return Final{}, err
	}
	if err := s.mustRun(ctx, byUserID, l); err != nil {
		return Final{}, err
	}
	// A fixed meeting is refused under the row lock below, together with a fixing
	// that started at the same moment.
	if !l.meeting.VotingOver(s.now()) {
		return Final{}, ErrVotingNotFinished
	}
	total := registry.CentiToM2(l.initiative.TotalAreaCenti)
	totalNum, totalDen, err := centi(total)
	if err != nil {
		return Final{}, err
	}

	f := Final{MeetingID: l.meeting.ID}
	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// The row lock orders the fixing against decisions entered at the same moment.
		if err := lockMeeting(ctx, tx, l.meeting.ID, "FOR UPDATE"); err != nil {
			return err
		}
		counted, err := s.countedBallots(ctx, tx, l.meeting.ID)
		if err != nil {
			return err
		}
		gis, err := s.gisResults(ctx, tx, l.meeting.ID)
		if err != nil {
			return err
		}
		if f.Result, err = tally(total, counted, gis, l.initiative.AgendaItems); err != nil {
			return err
		}
		f.Outcome = OutcomeNoQuorum
		if f.Result.QuorumReached {
			f.Outcome = OutcomeHeld
		}

		for _, item := range f.Result.Items {
			w, err := centiAll(item.ForM2, item.AgainstM2, item.AbstainM2)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO meeting_results (meeting_id, agenda_item_id, for_weight_num, for_weight_den,
				                             against_weight_num, against_weight_den, abstain_weight_num,
				                             abstain_weight_den, total_weight_num, total_weight_den,
				                             majority_rule, accepted, finalized_by)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::uuid)`,
				l.meeting.ID, item.AgendaItemID, w[0], w[1], w[2], w[3], w[4], w[5], totalNum, totalDen,
				item.MajorityRule, item.Accepted, byUserID); err != nil {
				return fmt.Errorf("result of question %d: %w", item.Position, err)
			}
		}

		participantsNum, participantsDen, err := centi(f.Result.ParticipantsM2)
		if err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			UPDATE meetings
			SET status = 'completed', outcome = $2, participants_weight_num = $3, participants_weight_den = $4,
			    finalized_by_user_id = $5::uuid, finalized_at = now(), updated_at = now()
			WHERE id = $1::uuid
			RETURNING finalized_at`,
			l.meeting.ID, f.Outcome, participantsNum, participantsDen, byUserID).Scan(&f.FinalizedAt); err != nil {
			return fmt.Errorf("fix meeting: %w", err)
		}

		if err := s.initiatives.SetStageTx(ctx, tx, l.initiative.ID,
			initiatives.StageMeeting, initiatives.StageCompleted, nil); err != nil {
			return err
		}

		return s.queue.EnqueueTx(ctx, tx, notify.Job{
			Type:     notify.TypeMeetingFinalized,
			DedupKey: notify.TypeMeetingFinalized + ":" + l.meeting.ID,
			Payload:  map[string]any{"meeting_id": l.meeting.ID},
		})
	})
	if err != nil {
		return Final{}, err
	}

	return f, nil
}

// lockMeeting locks the meeting row for the rest of the transaction and refuses a
// fixed meeting: whatever the caller changes must not land after the result.
func lockMeeting(ctx context.Context, tx pgx.Tx, meetingID, lock string) error {
	var stored string
	err := tx.QueryRow(ctx, `SELECT status FROM meetings WHERE id = $1::uuid `+lock, meetingID).Scan(&stored)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("lock meeting: %w", err)
	case stored == StatusCompleted:
		return ErrAlreadyFinalized
	}

	return nil
}

// centi turns an area in м² into the stored form: hundredths of м² as a fraction.
func centi(m2 *big.Rat) (num, den int64, err error) {
	c := new(big.Rat).Mul(m2, big.NewRat(100, 1))
	if !c.Num().IsInt64() || !c.Denom().IsInt64() {
		return 0, 0, fmt.Errorf("area %s м² does not fit the weight columns", m2.FloatString(2))
	}

	return c.Num().Int64(), c.Denom().Int64(), nil
}

// centiAll is centi for several areas: numerator and denominator of each in a row.
func centiAll(areas ...*big.Rat) ([]int64, error) {
	result := make([]int64, 0, 2*len(areas))
	for _, a := range areas {
		num, den, err := centi(a)
		if err != nil {
			return nil, err
		}
		result = append(result, num, den)
	}

	return result, nil
}
