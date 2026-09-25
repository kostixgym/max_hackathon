// Package rules implements the Housing Code thresholds of an owners' meeting (ОСС).
//
// All arithmetic is exact (math/big.Rat): shares like 1/3 must not be rounded,
// otherwise a vote that is exactly at "2/3 of all votes" could flip either way.
// Values are areas (м², any consistent unit); a vote weighs area × share.
package rules

import (
	"errors"
	"fmt"
	"math/big"
)

// Rule is a majority rule of a decision type (DecisionType.majority_rule).
type Rule string

const (
	// MajorityOfParticipants — more than half of the votes of the owners who took part
	// (ст. 46 ч. 1 ЖК, general rule).
	MajorityOfParticipants Rule = "majority_of_participants"
	// MoreThanHalfOfAll — more than 50% of all votes in the building.
	MoreThanHalfOfAll Rule = "more_than_half_of_all"
	// TwoThirdsOfAll — not less than two thirds of all votes in the building
	// (e.g. use of common property, which covers CCTV).
	TwoThirdsOfAll Rule = "two_thirds_of_all"
)

// ErrUnknownRule is returned for a rule that the calculator does not know.
var ErrUnknownRule = errors.New("unknown majority rule")

// ParseRule validates a rule read from storage.
func ParseRule(s string) (Rule, error) {
	switch r := Rule(s); r {
	case MajorityOfParticipants, MoreThanHalfOfAll, TwoThirdsOfAll:
		return r, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownRule, s)
	}
}

var (
	oneTenth  = big.NewRat(1, 10)
	oneHalf   = big.NewRat(1, 2)
	twoThirds = big.NewRat(2, 3)
)

// Thresholds are the reference values for a building with the given total area.
type Thresholds struct {
	// Demand — owners with at least this area may demand a meeting from the
	// management company (ст. 45 ч. 6 ЖК): support >= Demand.
	Demand *big.Rat
	// QuorumAbove — the meeting is valid when participants > QuorumAbove (ст. 45 ч. 3 ЖК).
	QuorumAbove *big.Rat
	// TwoThirds — "not less than 2/3 of all votes": votes for >= TwoThirds.
	TwoThirds *big.Rat
}

// ForTotal computes thresholds for the total area of the registry snapshot.
func ForTotal(total *big.Rat) Thresholds {
	return Thresholds{
		Demand:      mul(total, oneTenth),
		QuorumAbove: mul(total, oneHalf),
		TwoThirds:   mul(total, twoThirds),
	}
}

// DemandReached reports whether support is enough to demand a meeting (>= 10% of all).
func DemandReached(total, support *big.Rat) bool {
	return support.Cmp(mul(total, oneTenth)) >= 0
}

// QuorumReached reports whether the meeting is valid (> 50% of all votes took part).
func QuorumReached(total, participants *big.Rat) bool {
	return participants.Cmp(mul(total, oneHalf)) > 0
}

// Passed reports whether a question is accepted under the rule.
// Quorum is a property of the meeting as a whole and is checked separately
// with QuorumReached; without quorum no question is accepted.
func Passed(rule Rule, total, participants, votesFor *big.Rat) (bool, error) {
	switch rule {
	case MajorityOfParticipants:
		return votesFor.Cmp(mul(participants, oneHalf)) > 0, nil
	case MoreThanHalfOfAll:
		return votesFor.Cmp(mul(total, oneHalf)) > 0, nil
	case TwoThirdsOfAll:
		return votesFor.Cmp(mul(total, twoThirds)) >= 0, nil
	default:
		return false, fmt.Errorf("%w: %q", ErrUnknownRule, rule)
	}
}

func mul(a, b *big.Rat) *big.Rat {
	return new(big.Rat).Mul(a, b)
}
