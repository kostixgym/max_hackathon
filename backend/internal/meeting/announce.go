package meeting

// System-level read for the bot notifications: the same card the staff sees,
// without viewer checks — the worker has no user context (docs/plan-do-30-09.md, К3).

import (
	"context"
)

// BotView returns the meeting card for system notifications. Choices never
// appear here: the card shows dates, agenda and the ballot collection only.
func (s *Service) BotView(ctx context.Context, meetingID string) (View, error) {
	l, err := s.load(ctx, meetingID)
	if err != nil {
		return View{}, err
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
	}, nil
}
