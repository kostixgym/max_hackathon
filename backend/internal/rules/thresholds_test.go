package rules

import (
	"errors"
	"math/big"
	"testing"
)

func r(num, den int64) *big.Rat { return big.NewRat(num, den) }

func TestForTotal(t *testing.T) {
	th := ForTotal(r(3000, 1))
	if th.Demand.Cmp(r(300, 1)) != 0 || th.QuorumAbove.Cmp(r(1500, 1)) != 0 || th.TwoThirds.Cmp(r(2000, 1)) != 0 {
		t.Fatalf("unexpected thresholds: %v %v %v", th.Demand, th.QuorumAbove, th.TwoThirds)
	}
}

func TestDemandReached(t *testing.T) {
	total := r(3000, 1)
	if !DemandReached(total, r(300, 1)) {
		t.Fatal("exactly 10% must be enough (не менее 10%)")
	}
	if DemandReached(total, r(29999, 100)) {
		t.Fatal("below 10% must not be enough")
	}
}

func TestQuorumReached(t *testing.T) {
	total := r(3000, 1)
	if QuorumReached(total, r(1500, 1)) {
		t.Fatal("exactly 50% is not a quorum (нужно более 50%)")
	}
	if !QuorumReached(total, r(150001, 100)) {
		t.Fatal("more than 50% is a quorum")
	}
}

func TestPassed(t *testing.T) {
	total := r(3000, 1)
	cases := []struct {
		name                   string
		rule                   Rule
		participants, votesFor *big.Rat
		want                   bool
	}{
		{"2/3: exactly two thirds passes", TwoThirdsOfAll, r(2500, 1), r(2000, 1), true},
		{"2/3: just below fails", TwoThirdsOfAll, r(2500, 1), r(199999, 100), false},
		{"2/3 counts all votes, not participants", TwoThirdsOfAll, r(1600, 1), r(1600, 1), false},
		{">50% of all: exactly half fails", MoreThanHalfOfAll, r(2000, 1), r(1500, 1), false},
		{">50% of all: above half passes", MoreThanHalfOfAll, r(2000, 1), r(150001, 100), true},
		{"majority of participants: exactly half fails", MajorityOfParticipants, r(1600, 1), r(800, 1), false},
		{"majority of participants: above half passes", MajorityOfParticipants, r(1600, 1), r(80001, 100), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Passed(c.rule, total, c.participants, c.votesFor)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("Passed = %v, want %v", got, c.want)
			}
		})
	}
}

// Three co-owners with 1/3 each of a 90 м² flat: an exact sum must equal the whole flat.
func TestThirdsDoNotLoseArea(t *testing.T) {
	flat := r(90, 1)
	sum := new(big.Rat)
	for range 3 {
		sum.Add(sum, new(big.Rat).Mul(flat, r(1, 3)))
	}
	if sum.Cmp(flat) != 0 {
		t.Fatalf("sum of thirds = %v, want %v", sum, flat)
	}

	// Two thirds of all exactly at the boundary made of thirds must pass.
	total := r(90, 1)
	votesFor := new(big.Rat).Mul(flat, r(2, 3))
	ok, err := Passed(TwoThirdsOfAll, total, total, votesFor)
	if err != nil || !ok {
		t.Fatalf("exact 2/3 built from thirds must pass, got %v %v", ok, err)
	}
}

func TestParseRule(t *testing.T) {
	if _, err := ParseRule("two_thirds_of_all"); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRule("unanimous"); !errors.Is(err, ErrUnknownRule) {
		t.Fatalf("expected ErrUnknownRule, got %v", err)
	}
	if _, err := Passed(Rule("x"), r(1, 1), r(1, 1), r(1, 1)); !errors.Is(err, ErrUnknownRule) {
		t.Fatalf("expected ErrUnknownRule, got %v", err)
	}
}
