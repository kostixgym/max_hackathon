package meeting

import (
	"context"
	"fmt"
	"math/big"

	"maxhackathon/backend/internal/registry"
)

// Get returns the meeting card. It is seen by the staff running the meeting and by
// the verified residents of the house (решение 19): dates, agenda and the collection
// of the ballots, never how anyone voted.
func (s *Service) Get(ctx context.Context, meetingID, viewerID string) (View, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return View{}, err
	}
	admin, err := s.runs(ctx, viewerID, l)
	if err != nil {
		return View{}, err
	}
	if !admin {
		member, err := s.access.MayViewInitiatives(ctx, viewerID, l.house.ID)
		if err != nil {
			return View{}, err
		}
		if !member {
			return View{}, ErrNotMember
		}
	}

	owners, err := s.registry.SnapshotOwners(ctx, l.initiative.RegistryUploadID)
	if err != nil {
		return View{}, err
	}
	ballots, err := s.ballots(ctx, s.pool, l.meeting.ID)
	if err != nil {
		return View{}, err
	}

	return View{
		Meeting:   l.meeting,
		Title:     l.initiative.Title,
		House:     l.house,
		Chair:     officer(owners, l.meeting.ChairOwnerID),
		Secretary: officer(owners, l.meeting.SecretaryOwnerID),
		Agenda:    l.initiative.AgendaItems,
		Progress:  progress(ballots, l.initiative.TotalAreaCenti),
		IsAdmin:   admin,
	}, nil
}

// officer finds the chair or the secretary in the snapshot; the name is masked
// because every resident of the house sees the card.
func officer(owners []registry.SnapshotOwner, ownerID string) Officer {
	for _, o := range owners {
		if o.ID == ownerID {
			return Officer{OwnerID: o.ID, MaskedName: registry.MaskName(o.FullName, o.Kind)}
		}
	}

	return Officer{OwnerID: ownerID}
}

// TrackerRow is one ballot in the administrator's tracker: whose it is and whether
// it is in hand, never the choices.
type TrackerRow struct {
	BallotID        string
	PremiseNumber   string
	Entrance        *int
	OwnerMaskedName string
	WeightM2        *big.Rat
	Status          string
}

// Tracker is the collection of the ballots flat by flat.
type Tracker struct {
	BallotsTotal   int
	Received       int // in hand: received or counted
	Counted        int
	ParticipantsM2 *big.Rat
	Ballots        []TrackerRow
}

// Tracker returns the ballots in the order of the round of the flats: entrance,
// then flat number. Only the staff running the meeting sees it.
func (s *Service) Tracker(ctx context.Context, meetingID, viewerID string) (Tracker, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return Tracker{}, err
	}
	if err := s.mustRun(ctx, viewerID, l); err != nil {
		return Tracker{}, err
	}

	owners, err := s.registry.SnapshotOwners(ctx, l.initiative.RegistryUploadID)
	if err != nil {
		return Tracker{}, err
	}
	ballots, err := s.ballots(ctx, s.pool, l.meeting.ID)
	if err != nil {
		return Tracker{}, err
	}
	byOwner := make(map[string]ballot, len(ballots))
	for _, b := range ballots {
		byOwner[b.OwnerID] = b
	}

	p := progress(ballots, l.initiative.TotalAreaCenti)
	t := Tracker{
		BallotsTotal:   p.BallotsTotal,
		Received:       p.BallotsReceived,
		ParticipantsM2: p.ParticipantsM2,
		Ballots:        make([]TrackerRow, 0, len(ballots)),
	}
	for _, o := range owners {
		b, ok := byOwner[o.ID]
		if !ok {
			continue
		}
		if b.Status == BallotCounted {
			t.Counted++
		}
		t.Ballots = append(t.Ballots, TrackerRow{
			BallotID:        b.ID,
			PremiseNumber:   o.PremiseNumber,
			Entrance:        o.Entrance,
			OwnerMaskedName: registry.MaskName(o.FullName, o.Kind),
			WeightM2:        b.Weight.M2(),
			Status:          b.Status,
		})
	}
	if len(t.Ballots) != len(ballots) {
		return Tracker{}, fmt.Errorf("meeting %s: %d ballots, %d found in the snapshot",
			l.meeting.ID, len(ballots), len(t.Ballots))
	}

	return t, nil
}

// mustRun answers ErrStaffOnly to a user who does not run the meeting.
func (s *Service) mustRun(ctx context.Context, userID string, l loaded) error {
	runs, err := s.runs(ctx, userID, l)
	if err != nil {
		return err
	}
	if !runs {
		return ErrStaffOnly
	}

	return nil
}
