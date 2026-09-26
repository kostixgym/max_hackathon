package notify

import (
	"testing"
	"time"
)

func TestQuietHoursEnd(t *testing.T) {
	msk := time.FixedZone("MSK", 3*60*60)
	ekb := time.FixedZone("YEKT", 5*60*60)
	at := func(loc *time.Location, day, hour, minute int) time.Time {
		return time.Date(2026, time.September, day, hour, minute, 0, 0, loc)
	}

	cases := []struct {
		name  string
		now   time.Time
		loc   *time.Location
		quiet bool
		until time.Time
	}{
		{"day time", at(msk, 26, 14, 0), msk, false, time.Time{}},
		{"right before the quiet hours", at(msk, 26, 21, 59), msk, false, time.Time{}},
		{"quiet hours start at 22:00", at(msk, 26, 22, 0), msk, true, at(msk, 27, 9, 0)},
		{"after midnight", at(msk, 27, 3, 30), msk, true, at(msk, 27, 9, 0)},
		{"right before the morning", at(msk, 27, 8, 59), msk, true, at(msk, 27, 9, 0)},
		{"morning", at(msk, 27, 9, 0), msk, false, time.Time{}},
		// 20:00 in Moscow is 22:00 in Yekaterinburg: the house's local time decides.
		{"another time zone of the house", at(msk, 26, 20, 0), ekb, true, at(ekb, 27, 9, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			until, quiet := QuietHoursEnd(c.now, c.loc)
			if quiet != c.quiet || !until.Equal(c.until) {
				t.Fatalf("QuietHoursEnd(%s) = %s, %v; want %s, %v", c.now, until, quiet, c.until, c.quiet)
			}
		})
	}
}
