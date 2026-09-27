package meeting

// Read API for Дима (docs/plan-do-30-09.md, Г7): the meeting of an initiative for its
// card (Д4) and the data of the protocol PDF (Д5).

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"

	"maxhackathon/backend/internal/registry"
)

// Ref is the meeting of an initiative as its card links it.
type Ref struct {
	ID     string
	Status string
}

// ActiveMeeting returns the current meeting of the initiative: the latest one that is
// not canceled, with its status by the dates. A completed meeting is returned too, so
// the card leads to the result; false when the initiative has no meeting.
func (s *Service) ActiveMeeting(ctx context.Context, initiativeID string) (Ref, bool, error) {
	if !validID(initiativeID) {
		return Ref{}, false, nil
	}
	var r Ref
	var stored string
	var notice, starts, ends time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, status, notice_at, voting_starts_at, voting_ends_at
		FROM meetings
		WHERE initiative_id = $1::uuid AND status NOT IN ('canceled', 'canceling')
		ORDER BY attempt DESC
		LIMIT 1`, initiativeID).Scan(&r.ID, &stored, &notice, &starts, &ends)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ref{}, false, nil
	}
	if err != nil {
		return Ref{}, false, fmt.Errorf("active meeting: %w", err)
	}
	r.Status = effectiveStatus(stored, notice, starts, ends, s.now())

	return r, true, nil
}

// ProtocolPerson is the chair or the secretary as the protocol names them: in full.
type ProtocolPerson struct {
	OwnerID       string
	FullName      string
	PremiseNumber string
}

// Protocol is what the protocol of the meeting (Приказ Минстроя № 44/пр) is built of.
type Protocol struct {
	MeetingID       string
	InitiativeTitle string
	HouseAddress    string
	Attempt         int
	Form            string
	NoticeAt        time.Time
	VotingStartsAt  time.Time
	VotingEndsAt    time.Time
	FinalizedAt     time.Time
	Outcome         string
	Chair           ProtocolPerson
	Secretary       ProtocolPerson
	TotalM2         *big.Rat
	ParticipantsM2  *big.Rat
	QuorumReached   bool
	// Items are in the agenda order; the numbers come from the fixed result only.
	Items []ItemResult
}

// ProtocolData returns the data of the protocol of a fixed meeting, ErrNotFinalized
// before the result is fixed. It reads the write-once result, never recounts the
// ballots. Who may download the protocol is the caller's question.
func (s *Service) ProtocolData(ctx context.Context, meetingID string) (Protocol, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return Protocol{}, err
	}
	m := l.meeting
	if m.Status != StatusCompleted || m.Outcome == nil || m.FinalizedAt == nil {
		return Protocol{}, ErrNotFinalized
	}

	var participants registry.Weight
	if err := s.pool.QueryRow(ctx, `
		SELECT participants_weight_num, participants_weight_den FROM meetings WHERE id = $1::uuid`,
		m.ID).Scan(&participants.Num, &participants.Den); err != nil {
		return Protocol{}, fmt.Errorf("meeting participants: %w", err)
	}
	stored, err := s.results(ctx, m.ID)
	if err != nil {
		return Protocol{}, err
	}
	owners, err := s.registry.SnapshotOwners(ctx, l.initiative.RegistryUploadID)
	if err != nil {
		return Protocol{}, err
	}

	p := Protocol{
		MeetingID:       m.ID,
		InitiativeTitle: l.initiative.Title,
		HouseAddress:    l.house.Address,
		Attempt:         m.Attempt,
		Form:            m.Form,
		NoticeAt:        m.NoticeAt,
		VotingStartsAt:  m.VotingStartsAt,
		VotingEndsAt:    m.VotingEndsAt,
		FinalizedAt:     *m.FinalizedAt,
		Outcome:         *m.Outcome,
		Chair:           person(owners, m.ChairOwnerID),
		Secretary:       person(owners, m.SecretaryOwnerID),
		TotalM2:         registry.CentiToM2(l.initiative.TotalAreaCenti),
		ParticipantsM2:  participants.M2(),
		QuorumReached:   *m.Outcome == OutcomeHeld,
		Items:           make([]ItemResult, 0, len(l.initiative.AgendaItems)),
	}
	for _, item := range l.initiative.AgendaItems {
		r, ok := stored[item.ID]
		if !ok {
			return Protocol{}, fmt.Errorf("meeting %s: no fixed result for question %d", m.ID, item.Position)
		}
		r.Position, r.Text = item.Position, item.Text
		p.Items = append(p.Items, r)
	}

	return p, nil
}

// results reads the fixed result by agenda item id.
func (s *Service) results(ctx context.Context, meetingID string) (map[string]ItemResult, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT agenda_item_id::text, for_weight_num, for_weight_den, against_weight_num, against_weight_den,
		       abstain_weight_num, abstain_weight_den, majority_rule, accepted
		FROM meeting_results
		WHERE meeting_id = $1::uuid`, meetingID)
	if err != nil {
		return nil, fmt.Errorf("meeting results: %w", err)
	}
	defer rows.Close()

	result := map[string]ItemResult{}
	for rows.Next() {
		var r ItemResult
		var forW, againstW, abstainW registry.Weight
		if err := rows.Scan(&r.AgendaItemID, &forW.Num, &forW.Den, &againstW.Num, &againstW.Den,
			&abstainW.Num, &abstainW.Den, &r.MajorityRule, &r.Accepted); err != nil {
			return nil, fmt.Errorf("scan meeting result: %w", err)
		}
		r.ForM2, r.AgainstM2, r.AbstainM2 = forW.M2(), againstW.M2(), abstainW.M2()
		result[r.AgendaItemID] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("meeting results: %w", err)
	}

	return result, nil
}

// person finds the chair or the secretary in the snapshot with the full name.
func person(owners []registry.SnapshotOwner, ownerID string) ProtocolPerson {
	for _, o := range owners {
		if o.ID == ownerID {
			return ProtocolPerson{OwnerID: o.ID, FullName: o.FullName, PremiseNumber: o.PremiseNumber}
		}
	}

	return ProtocolPerson{OwnerID: ownerID}
}
