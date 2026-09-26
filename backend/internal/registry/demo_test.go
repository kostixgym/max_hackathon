package registry

import (
	"math/big"
	"testing"
)

func TestDemoPremises(t *testing.T) {
	premises := DemoPremises()

	if len(premises) != 61 {
		t.Fatalf("premises = %d, want 60 flats + 1 shop", len(premises))
	}
	if got := DemoTotalAreaCenti(premises); got != 300000 {
		t.Fatalf("total area = %d, want 300000 (3 000,00 м²)", got)
	}

	numbers := map[string]bool{}
	var coOwned, thirds, municipal, withPhone, withoutPhone int
	for _, p := range premises {
		if numbers[p.Number] {
			t.Fatalf("duplicate premise number %s", p.Number)
		}
		numbers[p.Number] = true

		if ShareSum(p.Owners).Cmp(big.NewRat(1, 1)) != 0 {
			t.Fatalf("premise %s: shares sum to %v, want 1", p.Number, ShareSum(p.Owners))
		}
		if len(p.Owners) == 2 {
			coOwned++
		}
		if len(p.Owners) == 3 {
			thirds++
		}
		for _, o := range p.Owners {
			if o.Kind == "municipality" {
				municipal++
			}
			if o.Kind == "person" {
				if o.Phone != "" {
					withPhone++
				} else {
					withoutPhone++
				}
			}
		}
	}

	// The demo must exercise the special cases from docs/01.
	if coOwned == 0 || thirds == 0 || municipal != 1 || withPhone == 0 || withoutPhone == 0 {
		t.Fatalf("special cases missing: 1/2=%d 1/3=%d municipal=%d phone=%d/%d", coOwned, thirds, municipal, withPhone, withoutPhone)
	}

	// Deterministic: the same call gives the same data.
	again := DemoPremises()
	if again[44].Owners[0].FullName != premises[44].Owners[0].FullName {
		t.Fatal("demo data must be deterministic")
	}
}
