package notify

import "time"

// Quiet hours of mailings: 22:00–09:00 by the local time of the house (docs/04,
// решение 29). Poll invitations and reminders wait for the morning; the important
// events of a meeting (its start and the outcome) go out at any time.
const (
	quietFrom  = 22
	quietUntil = 9
)

// QuietHoursEnd reports whether now falls into the quiet hours in loc and, if so,
// when they end: the job is then queued to run at that moment.
func QuietHoursEnd(now time.Time, loc *time.Location) (time.Time, bool) {
	local := now.In(loc)
	hour := local.Hour()
	if hour >= quietUntil && hour < quietFrom {
		return time.Time{}, false
	}

	day := local
	if hour >= quietFrom {
		day = local.AddDate(0, 0, 1)
	}

	return time.Date(day.Year(), day.Month(), day.Day(), quietUntil, 0, 0, 0, loc), true
}
