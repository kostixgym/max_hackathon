package registry

import (
	"errors"
	"math/big"
	"testing"
)

func TestParseAreaCenti(t *testing.T) {
	ok := map[string]int64{"52.3": 5230, "52,30": 5230, "52": 5200, " 0.01 ": 1, "1234.56": 123456}
	for in, want := range ok {
		got, err := ParseAreaCenti(in)
		if err != nil || got != want {
			t.Fatalf("ParseAreaCenti(%q) = %d, %v; want %d", in, got, err, want)
		}
	}

	for _, in := range []string{"", "0", "-5", "52.345", "abc", "52.", ".5", "+52.3", "52.+5", "52.-5", "5 2"} {
		if _, err := ParseAreaCenti(in); !errors.Is(err, ErrInvalidArea) {
			t.Fatalf("ParseAreaCenti(%q): expected ErrInvalidArea, got %v", in, err)
		}
	}
}

func TestWeight(t *testing.T) {
	// Кв. 45 · 52,30 м² · доля 1/2 → 26,15 м².
	w, err := NewWeight(5230, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if w.M2().Cmp(big.NewRat(2615, 100)) != 0 {
		t.Fatalf("weight = %v", w.M2())
	}
	if got := FormatM2(w.M2()); got != "26,15" {
		t.Fatalf("FormatM2 = %q", got)
	}

	// 1/3 of 52,30 м² is not a finite decimal: stays exact, rounds only for display.
	w3, _ := NewWeight(5230, 1, 3)
	if FormatM2(w3.M2()) != "17,43" {
		t.Fatalf("FormatM2(1/3) = %q", FormatM2(w3.M2()))
	}
	sum := new(big.Rat).Add(w3.M2(), w3.M2())
	sum.Add(sum, w3.M2())
	if sum.Cmp(CentiToM2(5230)) != 0 {
		t.Fatalf("three thirds = %v, want 52.3", sum)
	}

	for _, s := range [][2]int64{{0, 1}, {2, 1}, {1, 0}, {-1, 2}} {
		if _, err := NewWeight(5230, s[0], s[1]); !errors.Is(err, ErrInvalidShare) {
			t.Fatalf("share %d/%d: expected ErrInvalidShare, got %v", s[0], s[1], err)
		}
	}
}
