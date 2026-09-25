// Package poll is the «Опрос поддержки» module (docs/04, module 5): a poll
// without legal force that measures support in м² before an official meeting.
// A vote belongs to an owner, weighs area × share of the initiative's registry
// snapshot and can be changed while the poll is on (решения 1, 3, 54).
package poll

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/jackc/pgx/v5/pgxpool"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
)

// Polling reads the initiative (the initiatives module).
type Polling interface {
	Polling(ctx context.Context, id string) (initiatives.Polling, error)
}

// Owners resolves the caller's verified owner links (the access module).
type Owners interface {
	VerifiedOwnerIn(ctx context.Context, userID, houseID string) ([]access.OwnerLink, error)
}

// Snapshots reads registry snapshot facts.
type Snapshots interface {
	SnapshotTotal(ctx context.Context, uploadID string) (int64, error)
	OwnerWeightInSnapshot(ctx context.Context, uploadID, ownerID string) (num, den int64, err error)
}

// Domain errors.
var (
	// ErrPollClosed means the initiative is not on the poll stage.
	ErrPollClosed = errors.New("poll is closed")
	// ErrNotOwner means the caller has no verified owner link in the house.
	ErrNotOwner = errors.New("only verified owners may vote")
	// ErrNoWeight means the owner is absent from the initiative's snapshot.
	ErrNoWeight = errors.New("owner is not in the initiative registry snapshot")
)

// Choices of the poll (решение 4).
const (
	ChoiceFor     = "for"
	ChoiceAgainst = "against"
)

// Official channels of the survey (решение 62: asked only of those voting "for").
const (
	ChannelGosuslugi = "gosuslugi"
	ChannelPaper     = "paper"
)

// Store reads and writes poll_votes.
type Store struct {
	pool      *pgxpool.Pool
	polling   Polling
	owners    Owners
	snapshots Snapshots
}

// NewStore creates a poll store.
func NewStore(pool *pgxpool.Pool, polling Polling, owners Owners, snapshots Snapshots) *Store {
	return &Store{pool: pool, polling: polling, owners: owners, snapshots: snapshots}
}

// CastInput is one vote or vote change from the chat or the mini-app.
type CastInput struct {
	InitiativeID string
	UserID       string
	Choice       string // for / against
	// Survey (optional): how the owner plans to vote officially and whether they
	// are ready to help collect paper ballots.
	OfficialChannel string // "" / gosuslugi / paper
	WillingToHelp   bool
}

// CastResult echoes the counted weight back to the voter.
type CastResult struct {
	Choice        string
	PremiseNumber string
	WeightNum     int64 // hundredths of м² as an exact fraction Num/Den
	WeightDen     int64
}

// CastVote records or changes the vote of one owner. The weight is copied from
// the initiative's registry snapshot and never changes afterwards (решение 54).
func (s *Store) CastVote(ctx context.Context, in CastInput) (CastResult, error) {
	if in.Choice != ChoiceFor && in.Choice != ChoiceAgainst {
		return CastResult{}, fmt.Errorf("invalid choice %q", in.Choice)
	}

	initiative, err := s.polling.Polling(ctx, in.InitiativeID)
	if errors.Is(err, initiatives.ErrNotFound) {
		return CastResult{}, ErrPollClosed
	}
	if err != nil {
		return CastResult{}, err
	}
	if initiative.Stage != "poll" {
		return CastResult{}, fmt.Errorf("%w: stage %s", ErrPollClosed, initiative.Stage)
	}

	links, err := s.owners.VerifiedOwnerIn(ctx, in.UserID, initiative.HouseID)
	if errors.Is(err, access.ErrNoOwnerLink) {
		return CastResult{}, ErrNotOwner
	}
	if err != nil {
		return CastResult{}, err
	}

	channel := in.OfficialChannel
	if channel != "" && channel != ChannelGosuslugi && channel != ChannelPaper {
		return CastResult{}, fmt.Errorf("invalid official channel %q", channel)
	}
	// Инвариант 7 (docs/04): the survey exists only for "for" votes.
	willing := in.WillingToHelp && in.Choice == ChoiceFor
	if in.Choice == ChoiceAgainst {
		channel = ""
	}

	var channelValue any
	if channel != "" {
		channelValue = channel
	}

	// One vote row per owner record: a user with several premises votes with each
	// share, and the weights sum (docs/01, «Особые случаи»).
	result := CastResult{Choice: in.Choice}
	totalWeight := new(big.Rat)
	for _, link := range links {
		weightNum, weightDen, err := s.snapshots.OwnerWeightInSnapshot(ctx, initiative.RegistryUploadID, link.OwnerID)
		if errors.Is(err, registry.ErrNotFound) {
			return CastResult{}, ErrNoWeight
		}
		if err != nil {
			return CastResult{}, err
		}

		_, err = s.pool.Exec(ctx, `
			INSERT INTO poll_votes (initiative_id, owner_id, user_id, choice,
			                        weight_num, weight_den, official_channel, willing_to_help)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8)
			ON CONFLICT (initiative_id, owner_id) DO UPDATE
			SET choice = EXCLUDED.choice, user_id = EXCLUDED.user_id,
			    weight_num = EXCLUDED.weight_num, weight_den = EXCLUDED.weight_den,
			    official_channel = EXCLUDED.official_channel, willing_to_help = EXCLUDED.willing_to_help,
			    updated_at = now()`,
			in.InitiativeID, link.OwnerID, in.UserID, in.Choice, weightNum, weightDen, channelValue, willing)
		if err != nil {
			return CastResult{}, fmt.Errorf("cast vote: %w", err)
		}

		totalWeight.Add(totalWeight, new(big.Rat).SetFrac64(weightNum, weightDen))
		if result.PremiseNumber == "" {
			result.PremiseNumber = link.PremiseNumber
		} else {
			result.PremiseNumber += ", " + link.PremiseNumber
		}
	}

	result.WeightNum, result.WeightDen = fraction(totalWeight)

	return result, nil
}

// Progress is the live poll dashboard (решение 65: computed on the fly).
type Progress struct {
	InitiativeID string
	Stage        string
	TotalCenti   int64
	ForNum       int64 // exact sums, hundredths of м² as fractions
	ForDen       int64
	AgainstNum   int64
	AgainstDen   int64
	VotesFor     int
	VotesAgainst int
}

// ForM2 returns the "for" area in м² as an exact rational.
func (p Progress) ForM2() *big.Rat {
	return areaRat(p.ForNum, p.ForDen)
}

// AgainstM2 returns the "against" area in м² as an exact rational.
func (p Progress) AgainstM2() *big.Rat {
	return areaRat(p.AgainstNum, p.AgainstDen)
}

// areaRat guards against a zero denominator: an absent sum is 0/1, not 0/0
// (a hand-built or zero Progress must not panic on read). Weights are stored in
// hundredths of м², so the conversion to м² divides by 100.
func areaRat(num, den int64) *big.Rat {
	if den == 0 {
		return new(big.Rat)
	}

	return new(big.Rat).SetFrac64(num, den*100)
}

// TotalM2 returns the snapshot total area in м² as an exact rational.
func (p Progress) TotalM2() *big.Rat {
	if p.TotalCenti == 0 {
		return new(big.Rat)
	}

	return registry.CentiToM2(p.TotalCenti)
}

// Progress sums the votes of the initiative. Works on every stage: after the poll
// is frozen it shows the final poll result.
func (s *Store) Progress(ctx context.Context, initiativeID string) (Progress, error) {
	initiative, err := s.polling.Polling(ctx, initiativeID)
	if err != nil {
		return Progress{}, err
	}

	total, err := s.snapshots.SnapshotTotal(ctx, initiative.RegistryUploadID)
	if err != nil {
		return Progress{}, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT choice, weight_num, weight_den
		FROM poll_votes
		WHERE initiative_id = $1::uuid`, initiativeID)
	if err != nil {
		return Progress{}, fmt.Errorf("poll progress: %w", err)
	}
	defer rows.Close()

	progress := Progress{InitiativeID: initiative.ID, Stage: initiative.Stage, TotalCenti: total}
	forSum, againstSum := new(big.Rat), new(big.Rat)
	for rows.Next() {
		var choice string
		var num, den int64
		if err := rows.Scan(&choice, &num, &den); err != nil {
			return Progress{}, fmt.Errorf("scan poll vote: %w", err)
		}
		weight := new(big.Rat).SetFrac64(num, den) // hundredths of м²
		switch choice {
		case ChoiceFor:
			forSum.Add(forSum, weight)
			progress.VotesFor++
		case ChoiceAgainst:
			againstSum.Add(againstSum, weight)
			progress.VotesAgainst++
		}
	}
	if err := rows.Err(); err != nil {
		return Progress{}, fmt.Errorf("poll progress: %w", err)
	}

	progress.ForNum, progress.ForDen = fraction(forSum)
	progress.AgainstNum, progress.AgainstDen = fraction(againstSum)

	return progress, nil
}

func fraction(v *big.Rat) (int64, int64) {
	if v.Num().IsInt64() && v.Denom().IsInt64() {
		return v.Num().Int64(), v.Denom().Int64()
	}

	return 0, 1
}

// DemandReached reports whether the poll support is enough to demand a meeting
// from the management company (ст. 45 ч. 6 ЖК; решение 45).
func (p Progress) DemandReached() bool {
	return rules.DemandReached(registry.CentiToM2(p.TotalCenti), p.ForM2())
}

// Thresholds returns the reference values of the snapshot total.
func (p Progress) Thresholds() rules.Thresholds {
	return rules.ForTotal(registry.CentiToM2(p.TotalCenti))
}
