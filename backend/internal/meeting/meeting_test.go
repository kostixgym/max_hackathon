package meeting

import (
	"errors"
	"math/big"
	"testing"
	"time"

	"maxhackathon/backend/internal/registry"
)

var t0 = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func TestCheckDates(t *testing.T) {
	day := 24 * time.Hour
	tests := []struct {
		name                 string
		demo                 bool
		notice, starts, ends time.Time
		want                 string // "" is valid
	}{
		{"10 days exactly", false, t0, t0.Add(10 * day), t0.Add(20 * day), ""},
		{"a minute short of 10 days", false, t0, t0.Add(10*day - time.Minute), t0.Add(20 * day), DatesNoticePeriod},
		{"demo: voting right after the notice", true, t0, t0, t0.Add(time.Hour), ""},
		{"notice a few seconds late", false, t0.Add(-time.Minute), t0.Add(10 * day), t0.Add(11 * day), ""},
		{"notice in the past", true, t0.Add(-time.Hour), t0.Add(time.Hour), t0.Add(2 * time.Hour), DatesNoticeInPast},
		{"voting before the notice", true, t0.Add(time.Hour), t0, t0.Add(2 * time.Hour), DatesStartsBeforeNotice},
		{"voting ends when it starts", true, t0, t0.Add(time.Hour), t0.Add(time.Hour), DatesEndsBeforeStarts},
		{"missing date", false, t0, time.Time{}, t0.Add(time.Hour), DatesRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDates(t0, tt.demo, tt.notice, tt.starts, tt.ends)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("err = %v, want valid", err)
				}

				return
			}
			var de *DatesError
			if !errors.As(err, &de) || de.Reason != tt.want || !errors.Is(err, ErrInvalidDates) {
				t.Fatalf("err = %v, want reason %s", err, tt.want)
			}
		})
	}
}

func TestEffectiveStatus(t *testing.T) {
	notice, starts, ends := t0, t0.Add(10*24*time.Hour), t0.Add(20*24*time.Hour)
	tests := []struct {
		stored string
		now    time.Time
		want   string
	}{
		{StatusPreparation, notice.Add(-time.Second), StatusPreparation},
		{StatusPreparation, notice, StatusNotice},
		{StatusPreparation, starts, StatusVoting},
		{StatusPreparation, ends.Add(-time.Second), StatusVoting},
		{StatusPreparation, ends, StatusCounting},
		{StatusCompleted, starts, StatusCompleted},
		{StatusCanceled, notice.Add(-time.Hour), StatusCanceled},
	}
	for _, tt := range tests {
		if got := effectiveStatus(tt.stored, notice, starts, ends, tt.now); got != tt.want {
			t.Errorf("effectiveStatus(%s, %v) = %s, want %s", tt.stored, tt.now, got, tt.want)
		}
	}
}

func TestOfficersValid(t *testing.T) {
	owners := []registry.SnapshotOwner{{Owner: registry.Owner{ID: "a"}}, {Owner: registry.Owner{ID: "b"}}}
	for _, c := range []struct {
		chair, secretary string
		want             bool
	}{
		{"a", "b", true},
		{"a", "a", false}, // one person cannot sign twice
		{"a", "x", false}, // not an owner of the snapshot
	} {
		if got := officersValid(owners, c.chair, c.secretary); got != c.want {
			t.Errorf("officersValid(%s, %s) = %v, want %v", c.chair, c.secretary, got, c.want)
		}
	}
}

// Progress counts only the ballots in hand, with exact 1/3 shares.
func TestProgress(t *testing.T) {
	third := registry.Weight{Num: 4800, Den: 3} // 48.00 м² × 1/3
	ballots := []ballot{
		{Status: BallotPaperReceived, Weight: third},
		{Status: BallotCounted, Weight: third},
		{Status: BallotNotVoted, Weight: third},
		{Status: BallotCounted, Weight: registry.Weight{Num: 5230, Den: 1}},
	}
	p := progress(ballots, 300000)
	want := new(big.Rat).Add(big.NewRat(32, 1), big.NewRat(5230, 100)) // 16 + 16 + 52.30
	if p.BallotsTotal != 4 || p.BallotsReceived != 3 || p.ParticipantsM2.Cmp(want) != 0 {
		t.Fatalf("progress = %d/%d, %s м²; want 3/4, %s", p.BallotsReceived, p.BallotsTotal,
			p.ParticipantsM2.FloatString(2), want.FloatString(2))
	}
	if p.TotalM2.Cmp(big.NewRat(3000, 1)) != 0 || p.QuorumAboveM2.Cmp(big.NewRat(1500, 1)) != 0 {
		t.Fatalf("total %s, quorum above %s", p.TotalM2.FloatString(2), p.QuorumAboveM2.FloatString(2))
	}
}
