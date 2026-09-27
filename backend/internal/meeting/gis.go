package meeting

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
)

// GISResultEntry is the official online aggregate for one agenda question.
type GISResultEntry struct {
	AgendaItemID string
	ForM2        *big.Rat
	AgainstM2    *big.Rat
	AbstainM2    *big.Rat
}

// GISResults are the official online aggregates manually copied from GIS ЖКХ.
// The common participant area is stored once on the meeting.
type GISResults struct {
	Entries              []GISResultEntry
	OnlineParticipantsM2 *big.Rat
}

// RecordGISResults replaces the official online aggregates before the result is
// fixed. The complete agenda and the common participant area are written in one
// transaction, ordered against Finalize by the meeting row lock.
func (s *Service) RecordGISResults(
	ctx context.Context,
	meetingID, byUserID string,
	input GISResults,
) (GISResults, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return GISResults{}, err
	}
	if err := s.mustRun(ctx, byUserID, l); err != nil {
		return GISResults{}, err
	}
	if l.meeting.Form != FormGISElectronic {
		return GISResults{}, ErrGISResultsNotAllowed
	}
	if l.meeting.Status == StatusCompleted {
		return GISResults{}, ErrAlreadyFinalized
	}
	if !l.meeting.VotingOver(s.now()) {
		return GISResults{}, ErrVotingNotFinished
	}
	if len(input.Entries) == 0 {
		return GISResults{}, fmt.Errorf("%w: no agenda entries", ErrInvalidGISResults)
	}

	total := registry.CentiToM2(l.initiative.TotalAreaCenti)
	ordered, err := orderGISResults(l.initiative.AgendaItems, input.Entries, input.OnlineParticipantsM2, total)
	if err != nil {
		return GISResults{}, err
	}
	participantsNum, participantsDen, err := centi(input.OnlineParticipantsM2)
	if err != nil {
		return GISResults{}, fmt.Errorf("%w: online participants: %v", ErrInvalidGISResults, err)
	}

	err = s.tm.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := lockMeeting(ctx, tx, l.meeting.ID, "FOR UPDATE"); err != nil {
			return err
		}
		for _, entry := range ordered {
			weights, err := centiAll(entry.ForM2, entry.AgainstM2, entry.AbstainM2)
			if err != nil {
				return fmt.Errorf("%w: question %s: %v", ErrInvalidGISResults, entry.AgendaItemID, err)
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO gis_result_entries (
					meeting_id, agenda_item_id, for_weight_num, for_weight_den,
					against_weight_num, against_weight_den, abstain_weight_num,
					abstain_weight_den, entered_by_user_id
				)
				VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, $7, $8, $9::uuid)
				ON CONFLICT (meeting_id, agenda_item_id) DO UPDATE
				SET for_weight_num = EXCLUDED.for_weight_num,
				    for_weight_den = EXCLUDED.for_weight_den,
				    against_weight_num = EXCLUDED.against_weight_num,
				    against_weight_den = EXCLUDED.against_weight_den,
				    abstain_weight_num = EXCLUDED.abstain_weight_num,
				    abstain_weight_den = EXCLUDED.abstain_weight_den,
				    entered_by_user_id = EXCLUDED.entered_by_user_id,
				    updated_at = now()`,
				l.meeting.ID, entry.AgendaItemID,
				weights[0], weights[1], weights[2], weights[3], weights[4], weights[5], byUserID); err != nil {
				return fmt.Errorf("record GIS result for %s: %w", entry.AgendaItemID, err)
			}
		}

		if _, err := tx.Exec(ctx, `
			UPDATE meetings
			SET online_participants_weight_num = $2,
			    online_participants_weight_den = $3,
			    updated_at = now()
			WHERE id = $1::uuid`,
			l.meeting.ID, participantsNum, participantsDen); err != nil {
			return fmt.Errorf("record GIS participants: %w", err)
		}

		return nil
	})
	if err != nil {
		return GISResults{}, err
	}

	return GISResults{
		Entries:              cloneGISEntries(ordered),
		OnlineParticipantsM2: new(big.Rat).Set(input.OnlineParticipantsM2),
	}, nil
}

// orderGISResults validates a complete set of aggregates and returns it in agenda
// order. Empty results are accepted only as the internal representation of a meeting
// where the GIS data has not been entered yet.
func orderGISResults(
	agenda []initiatives.AgendaItem,
	entries []GISResultEntry,
	onlineParticipants, total *big.Rat,
) ([]GISResultEntry, error) {
	if onlineParticipants == nil || total == nil || onlineParticipants.Sign() < 0 || total.Sign() <= 0 {
		return nil, fmt.Errorf("%w: invalid participant or total area", ErrInvalidGISResults)
	}
	if onlineParticipants.Cmp(total) > 0 {
		return nil, fmt.Errorf("%w: online participants exceed the registry area", ErrInvalidGISResults)
	}
	if len(entries) == 0 && onlineParticipants.Sign() == 0 {
		return []GISResultEntry{}, nil
	}

	byItem := make(map[string]GISResultEntry, len(entries))
	for _, entry := range entries {
		if entry.ForM2 == nil || entry.AgainstM2 == nil || entry.AbstainM2 == nil ||
			entry.ForM2.Sign() < 0 || entry.AgainstM2.Sign() < 0 || entry.AbstainM2.Sign() < 0 {
			return nil, fmt.Errorf("%w: negative or missing area for question %s", ErrInvalidGISResults, entry.AgendaItemID)
		}
		if _, exists := byItem[entry.AgendaItemID]; exists {
			return nil, fmt.Errorf("%w: question %s occurs twice", ErrInvalidGISResults, entry.AgendaItemID)
		}
		sum := new(big.Rat).Add(entry.ForM2, entry.AgainstM2)
		sum.Add(sum, entry.AbstainM2)
		if sum.Cmp(onlineParticipants) != 0 {
			return nil, fmt.Errorf("%w: question %s sums to %s, participants are %s",
				ErrInvalidGISResults, entry.AgendaItemID, sum.RatString(), onlineParticipants.RatString())
		}
		byItem[entry.AgendaItemID] = entry
	}
	if len(byItem) != len(agenda) {
		return nil, fmt.Errorf("%w: %d entries for %d questions", ErrInvalidGISResults, len(byItem), len(agenda))
	}

	ordered := make([]GISResultEntry, 0, len(agenda))
	for _, item := range agenda {
		entry, ok := byItem[item.ID]
		if !ok {
			return nil, fmt.Errorf("%w: no result for question %d", ErrInvalidGISResults, item.Position)
		}
		ordered = append(ordered, GISResultEntry{
			AgendaItemID: entry.AgendaItemID,
			ForM2:        new(big.Rat).Set(entry.ForM2),
			AgainstM2:    new(big.Rat).Set(entry.AgainstM2),
			AbstainM2:    new(big.Rat).Set(entry.AbstainM2),
		})
	}

	return ordered, nil
}

// gisResults reads official online aggregates using either the pool or the current
// transaction. A NULL participant area means that GIS results have not been entered.
func (s *Service) gisResults(ctx context.Context, q querier, meetingID string) (GISResults, error) {
	result := GISResults{Entries: []GISResultEntry{}, OnlineParticipantsM2: new(big.Rat)}
	var participantsNum, participantsDen *int64
	err := q.QueryRow(ctx, `
		SELECT online_participants_weight_num, online_participants_weight_den
		FROM meetings
		WHERE id = $1::uuid`, meetingID).Scan(&participantsNum, &participantsDen)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return GISResults{}, ErrNotFound
	case err != nil:
		return GISResults{}, fmt.Errorf("GIS participants: %w", err)
	case participantsNum != nil && participantsDen != nil:
		result.OnlineParticipantsM2 = (registry.Weight{Num: *participantsNum, Den: *participantsDen}).M2()
	}

	rows, err := q.Query(ctx, `
		SELECT g.agenda_item_id::text,
		       g.for_weight_num, g.for_weight_den,
		       g.against_weight_num, g.against_weight_den,
		       g.abstain_weight_num, g.abstain_weight_den
		FROM gis_result_entries g
		WHERE g.meeting_id = $1::uuid
		ORDER BY g.agenda_item_id`, meetingID)
	if err != nil {
		return GISResults{}, fmt.Errorf("GIS results: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var entry GISResultEntry
		var forW, againstW, abstainW registry.Weight
		if err := rows.Scan(&entry.AgendaItemID, &forW.Num, &forW.Den, &againstW.Num, &againstW.Den,
			&abstainW.Num, &abstainW.Den); err != nil {
			return GISResults{}, fmt.Errorf("scan GIS result: %w", err)
		}
		entry.ForM2, entry.AgainstM2, entry.AbstainM2 = forW.M2(), againstW.M2(), abstainW.M2()
		result.Entries = append(result.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return GISResults{}, fmt.Errorf("GIS results: %w", err)
	}

	return result, nil
}

func cloneGISEntries(entries []GISResultEntry) []GISResultEntry {
	result := make([]GISResultEntry, 0, len(entries))
	for _, entry := range entries {
		result = append(result, GISResultEntry{
			AgendaItemID: entry.AgendaItemID,
			ForM2:        new(big.Rat).Set(entry.ForM2),
			AgainstM2:    new(big.Rat).Set(entry.AgainstM2),
			AbstainM2:    new(big.Rat).Set(entry.AbstainM2),
		})
	}

	return result
}
