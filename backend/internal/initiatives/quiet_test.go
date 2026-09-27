package initiatives

import (
	"testing"
	"time"
)

// QUIET_HOURS=false sends night mailings at once; by default the night waits for 09:00.
func TestQuietHoursSwitch(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	night := time.Date(2026, 9, 27, 23, 30, 0, 0, msk)

	var s Service
	if until, quiet := s.quietHoursEnd(night, msk); !quiet || !until.Equal(time.Date(2026, 9, 28, 9, 0, 0, 0, msk)) {
		t.Fatalf("default: until %v, quiet %v; want quiet until 09:00", until, quiet)
	}

	s.SetQuietHours(false)
	if until, quiet := s.quietHoursEnd(night, msk); quiet || !until.IsZero() {
		t.Fatalf("off: until %v, quiet %v; want to send now", until, quiet)
	}

	s.SetQuietHours(true)
	if _, quiet := s.quietHoursEnd(night, msk); !quiet {
		t.Fatal("back on: the night must be quiet again")
	}
}
