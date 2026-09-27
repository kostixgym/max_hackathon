package meeting

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"maxhackathon/backend/internal/initiatives"
)

var testAgenda = []initiatives.AgendaItem{
	{ID: "officers", Position: 1, Text: "Избрать председателя и секретаря", MajorityRule: "majority_of_participants"},
	{ID: "cameras", Position: 2, Text: "Установить видеонаблюдение", MajorityRule: "two_thirds_of_all"},
}

// counted builds a counted ballot of the weight with one choice on every question.
func counted(weight *big.Rat, officers, cameras string) countedBallot {
	return countedBallot{WeightM2: weight, Choices: map[string]string{"officers": officers, "cameras": cameras}}
}

func rat(num, den int64) *big.Rat { return big.NewRat(num, den) }

func TestTally(t *testing.T) {
	tests := []struct {
		name            string
		total           *big.Rat
		ballots         []countedBallot
		quorum          bool
		officers, video bool
	}{
		{
			// Three co-owners of 1/3 each sum to exactly the whole flat: 1 of 1.5 м² is the
			// quorum and exactly 2/3 of all, not 0.999… of it.
			name:  "shares 1/3 sum exactly, 2/3 exactly passes",
			total: rat(3, 2),
			ballots: []countedBallot{
				counted(rat(1, 3), ChoiceFor, ChoiceFor), counted(rat(1, 3), ChoiceFor, ChoiceFor),
				counted(rat(1, 3), ChoiceFor, ChoiceFor),
			},
			quorum: true, officers: true, video: true,
		},
		{
			name:  "a hair below 2/3 of all fails",
			total: rat(3000, 1),
			ballots: []countedBallot{
				counted(new(big.Rat).Sub(rat(2000, 1), rat(1, 100)), ChoiceFor, ChoiceFor),
				counted(rat(500, 1), ChoiceFor, ChoiceAgainst),
			},
			quorum: true, officers: true, video: false,
		},
		{
			name:    "exactly half of the area is no quorum",
			total:   rat(3000, 1),
			ballots: []countedBallot{counted(rat(1500, 1), ChoiceFor, ChoiceFor)},
			quorum:  false, officers: false, video: false,
		},
		{
			// 30 «for» of 60 participants is not more than half: abstentions count against.
			name:  "abstentions under the majority of participants",
			total: rat(100, 1),
			ballots: []countedBallot{
				counted(rat(30, 1), ChoiceFor, ChoiceFor), counted(rat(10, 1), ChoiceAgainst, ChoiceAgainst),
				counted(rat(20, 1), ChoiceAbstain, ChoiceAbstain),
			},
			quorum: true, officers: false, video: false,
		},
		{
			name:  "shares 1/2 of co-owners who decide differently",
			total: rat(100, 1),
			ballots: []countedBallot{
				counted(rat(26, 1), ChoiceFor, ChoiceFor), counted(rat(26, 1), ChoiceAgainst, ChoiceAgainst),
				counted(rat(1, 2), ChoiceFor, ChoiceFor),
			},
			quorum: true, officers: true, video: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := tally(tt.total, tt.ballots, testAgenda)
			if err != nil {
				t.Fatal(err)
			}
			if r.QuorumReached != tt.quorum || r.Items[0].Accepted != tt.officers || r.Items[1].Accepted != tt.video {
				t.Fatalf("quorum %v, officers %v, video %v; want %v, %v, %v (participants %s, for %s)",
					r.QuorumReached, r.Items[0].Accepted, r.Items[1].Accepted, tt.quorum, tt.officers, tt.video,
					r.ParticipantsM2.FloatString(4), r.Items[1].ForM2.FloatString(4))
			}
			// Every participant decided on every question: the three sums add up to them.
			for _, item := range r.Items {
				sum := new(big.Rat).Add(item.ForM2, item.AgainstM2)
				if sum.Add(sum, item.AbstainM2).Cmp(r.ParticipantsM2) != 0 {
					t.Fatalf("question %d: sums %s ≠ participants %s", item.Position, sum.FloatString(4),
						r.ParticipantsM2.FloatString(4))
				}
			}
		})
	}

	if _, err := tally(rat(100, 1), nil, []initiatives.AgendaItem{{ID: "x", MajorityRule: "unanimous"}}); err == nil {
		t.Fatal("an unknown majority rule must be an error")
	}
}

// The demo fill reaches the quorum and 2/3 of all on a house like the demo one, keeps
// counted ballots and does nothing the second time.
func TestFillPlan(t *testing.T) {
	total := rat(3000, 1)
	candidates := make([]fillBallot, 0, 60)
	for i := range 60 {
		candidates = append(candidates, fillBallot{ID: fmt.Sprint("b", i), WeightM2: rat(50, 1)})
	}

	check := func(t *testing.T, already []countedBallot, plan []filled) []countedBallot {
		t.Helper()
		all := append([]countedBallot{}, already...)
		weights := map[string]*big.Rat{}
		for _, c := range candidates {
			weights[c.ID] = c.WeightM2
		}
		for _, p := range plan {
			all = append(all, countedBallot{ID: p.ID, WeightM2: weights[p.ID], Choices: p.Choices})
		}
		r, err := tally(total, all, testAgenda)
		if err != nil {
			t.Fatal(err)
		}
		if r.ParticipantsM2.Cmp(rat(2400, 1)) < 0 || !r.QuorumReached || !r.Items[0].Accepted || !r.Items[1].Accepted {
			t.Fatalf("participants %s, quorum %v, accepted %v/%v; want ≥ 2400, both accepted",
				r.ParticipantsM2.FloatString(2), r.QuorumReached, r.Items[0].Accepted, r.Items[1].Accepted)
		}
		// Not everyone agrees: the demo shows real «against» and «abstain» too.
		if r.Items[1].AgainstM2.Sign() == 0 || r.Items[1].AbstainM2.Sign() == 0 {
			t.Fatalf("no against or abstain: %s / %s", r.Items[1].AgainstM2.FloatString(2), r.Items[1].AbstainM2.FloatString(2))
		}

		return all
	}

	t.Run("empty meeting", func(t *testing.T) {
		plan := fillPlan(total, candidates, nil, testAgenda)
		if len(plan) != 48 { // 48 × 50 = 2400 м² = 80%
			t.Fatalf("filled %d ballots, want 48", len(plan))
		}
		all := check(t, nil, plan)

		// The second call finds the target reached and fills nothing.
		rest := candidates[len(plan):]
		if again := fillPlan(total, rest, all, testAgenda); len(again) != 0 {
			t.Fatalf("second fill: %d ballots, want none", len(again))
		}
	})

	t.Run("after ballots entered by hand against", func(t *testing.T) {
		already := []countedBallot{
			{ID: "hand-1", WeightM2: rat(50, 1), Choices: map[string]string{"officers": ChoiceAgainst, "cameras": ChoiceAgainst}},
			{ID: "hand-2", WeightM2: rat(50, 1), Choices: map[string]string{"officers": ChoiceAgainst, "cameras": ChoiceAgainst}},
		}
		plan := fillPlan(total, candidates, already, testAgenda)
		if len(plan) != 46 {
			t.Fatalf("filled %d ballots, want 46 next to the 2 counted", len(plan))
		}
		check(t, already, plan)
	})
}

func TestOrderDecisions(t *testing.T) {
	good := []Decision{{AgendaItemID: "cameras", Choice: ChoiceAgainst}, {AgendaItemID: "officers", Choice: ChoiceFor}}
	ordered, err := orderDecisions(testAgenda, good)
	if err != nil || ordered[0].AgendaItemID != "officers" || ordered[1].Choice != ChoiceAgainst {
		t.Fatalf("ordered = %+v, %v; want the agenda order", ordered, err)
	}

	for name, decisions := range map[string][]Decision{
		"missing question": {{AgendaItemID: "officers", Choice: ChoiceFor}},
		"question twice": {{AgendaItemID: "officers", Choice: ChoiceFor}, {AgendaItemID: "officers", Choice: ChoiceAgainst},
			{AgendaItemID: "cameras", Choice: ChoiceFor}},
		"unknown choice":   {{AgendaItemID: "officers", Choice: "yes"}, {AgendaItemID: "cameras", Choice: ChoiceFor}},
		"foreign question": {{AgendaItemID: "officers", Choice: ChoiceFor}, {AgendaItemID: "other", Choice: ChoiceFor}},
	} {
		if _, err := orderDecisions(testAgenda, decisions); !errors.Is(err, ErrInvalidDecisions) {
			t.Errorf("%s: %v, want ErrInvalidDecisions", name, err)
		}
	}
}
