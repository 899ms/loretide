package topicplanning

import (
	"fmt"
	"time"
)

// Date is a calendar date in a node's own time zone: a year, month and day,
// with no clock and no zone attached.
//
// Day arithmetic goes through time.Date at noon UTC and reads the calendar
// fields back. It never multiplies 24*time.Hour: the day a zone leaves or
// enters daylight saving time is not 24 hours long, and "start minus n days"
// computed in hours lands on the wrong date once it crosses one of those days
// near midnight (specs/033 FR-015).
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseDate reads a strict YYYY-MM-DD calendar date. 2026-02-30 is refused
// rather than normalised to March.
func ParseDate(value string) (Date, bool) {
	if len(value) != len("2006-01-02") {
		return Date{}, false
	}
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		return Date{}, false
	}
	return Date{Year: parsed.Year(), Month: parsed.Month(), Day: parsed.Day()}, true
}

func (d Date) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

func (d Date) noonUTC() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.UTC)
}

func dateOf(t time.Time) Date {
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// AddDays moves the date by n calendar days (n may be negative).
func (d Date) AddDays(n int) Date {
	return dateOf(time.Date(d.Year, d.Month, d.Day+n, 12, 0, 0, 0, time.UTC))
}

// Compare returns -1, 0 or +1 as d is before, equal to or after other.
func (d Date) Compare(other Date) int {
	return d.noonUTC().Compare(other.noonUTC())
}

// DaysUntil is the number of calendar days from d to other; negative when
// other is earlier. Both sides are noon UTC, where every day is 24 hours, so
// the division is exact.
func (d Date) DaysUntil(other Date) int {
	return int(other.noonUTC().Sub(d.noonUTC()) / (24 * time.Hour))
}

// Today is the calendar date that the instant now falls on in loc. The same
// instant is 11-11 in Shanghai and 11-10 in Los Angeles, and a node's phase is
// judged by its own zone, not by the server's or by UTC (FR-014).
func Today(now time.Time, loc *time.Location) Date {
	return dateOf(now.In(loc))
}

// PreparationStartsOn is startsOn minus leadDays calendar days. The second
// result is false when no lead time was set: "not set" has no preparation
// start, and must not be read as zero (FR-013).
func PreparationStartsOn(startsOn Date, leadDays *int) (Date, bool) {
	if leadDays == nil {
		return Date{}, false
	}
	return startsOn.AddDays(-*leadDays), true
}

// NodePhase is where a node stands relative to "today" in its own zone.
type NodePhase string

const (
	PhaseBeforePreparation      NodePhase = "before_preparation"
	PhasePreparing              NodePhase = "preparing"
	PhaseLive                   NodePhase = "live"
	PhaseEnded                  NodePhase = "ended"
	PhaseBeforeStartUnknownLead NodePhase = "before_start_unknown_lead"
)

// Phase places a node against today (FR-016):
//
//   - after ends_on                  -> ended
//   - starts_on through ends_on      -> live
//   - before start, lead not set     -> before_start_unknown_lead
//   - on or after preparation start  -> preparing
//   - otherwise                      -> before_preparation
func Phase(today, startsOn, endsOn Date, leadDays *int) NodePhase {
	if today.Compare(endsOn) > 0 {
		return PhaseEnded
	}
	if today.Compare(startsOn) >= 0 {
		return PhaseLive
	}
	prep, ok := PreparationStartsOn(startsOn, leadDays)
	if !ok {
		return PhaseBeforeStartUnknownLead
	}
	if today.Compare(prep) >= 0 {
		return PhasePreparing
	}
	return PhaseBeforePreparation
}
