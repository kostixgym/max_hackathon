package demand

// Announce: system facts for the bot notification about a delivered demand
// (К3). No viewer checks — the worker is trusted, the access rule already held
// when the demand was created (docs/04, решение 79).

import (
	"context"
	"math/big"
	"time"
)

// Announce is what the bot tells the staff about a delivered demand.
type Announce struct {
	InitiativeID    string
	HouseID         string
	Title           string
	HouseAddress    string
	OrgName         string
	InitiatorUserID *string
	SupportM2       string // м², строка с точкой
	ThresholdM2     string
	DeliveredAt     time.Time
	UKDueAt         time.Time
}

// Announce returns the facts of the delivered demand.
func (s *Service) Announce(ctx context.Context, demandID string) (Announce, error) {
	var a Announce
	d, err := s.load(ctx, demandID)
	if err != nil {
		return a, err
	}

	in, err := s.initiatives.Details(ctx, d.InitiativeID)
	if err != nil {
		return a, err
	}
	house, err := s.registry.House(ctx, in.HouseID)
	if err != nil {
		return a, err
	}
	var orgName string
	if house.OrgID != nil {
		if orgs, err := s.registry.OrgsByIDs(ctx, []string{*house.OrgID}); err == nil && len(orgs) == 1 {
			orgName = orgs[0].Name
		}
	}

	progress, err := s.polls.Progress(ctx, d.InitiativeID)
	if err != nil {
		return a, err
	}
	thresholds := progress.Thresholds()

	// Поддержка — та, что зафиксирована в требовании на момент создания (решение
	// о фиксации дроби), а не живые цифры опроса: PDF и уведомление совпадают.
	support := new(big.Rat).SetFrac64(d.SupportNum, d.SupportDen)
	support.Quo(support, big.NewRat(100, 1))

	deliveredAt := time.Time{}
	if d.DeliveredAt != nil {
		deliveredAt = *d.DeliveredAt
	}
	dueAt := time.Time{}
	if d.UKDueAt != nil {
		dueAt = *d.UKDueAt
	}

	return Announce{
		InitiativeID:    d.InitiativeID,
		HouseID:         in.HouseID,
		Title:           in.Title,
		HouseAddress:    house.Address,
		OrgName:         orgName,
		InitiatorUserID: in.InitiatorUserID,
		SupportM2:       support.FloatString(2),
		ThresholdM2:     thresholds.Demand.FloatString(2),
		DeliveredAt:     deliveredAt,
		UKDueAt:         dueAt,
	}, nil
}
