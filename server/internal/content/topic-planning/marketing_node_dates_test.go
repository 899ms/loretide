package topicplanning

import (
	"testing"
	"time"
)

// Contract §3.2, spec Edge Cases and SC-003 (D14-V02).

func mustDate(t *testing.T, value string) Date {
	t.Helper()
	d, ok := ParseDate(value)
	if !ok {
		t.Fatalf("bad test date %q", value)
	}
	return d
}

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := LoadNodeLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func lead(n int) *int { return &n }

// The same injected instant is a different calendar day in two zones, and the
// phase follows the node's zone - not UTC and not the server's zone.
func TestTodayIsTheNodesZoneNotUTC(t *testing.T) {
	now := time.Date(2026, 11, 10, 16, 30, 0, 0, time.UTC)
	starts, ends := mustDate(t, "2026-11-11"), mustDate(t, "2026-11-11")

	shanghai := Today(now, mustLocation(t, "Asia/Shanghai"))
	if shanghai.String() != "2026-11-11" {
		t.Fatalf("Shanghai today = %s, want 2026-11-11", shanghai)
	}
	if got := Phase(shanghai, starts, ends, lead(14)); got != PhaseLive {
		t.Fatalf("Shanghai phase = %s, want live", got)
	}

	losAngeles := Today(now, mustLocation(t, "America/Los_Angeles"))
	if losAngeles.String() != "2026-11-10" {
		t.Fatalf("Los Angeles today = %s, want 2026-11-10", losAngeles)
	}
	if got := Phase(losAngeles, starts, ends, lead(14)); got != PhasePreparing {
		t.Fatalf("Los Angeles phase = %s, want preparing", got)
	}
}

func TestComputeTimingReportsTheDayAndZoneItUsed(t *testing.T) {
	now := time.Date(2026, 11, 10, 16, 30, 0, 0, time.UTC)
	timing, err := ComputeTiming(now, NodeContent{
		StartsOn: "2026-11-11", EndsOn: "2026-11-11", Timezone: "America/Los_Angeles", LeadDays: lead(14),
	})
	if err != nil {
		t.Fatal(err)
	}
	if timing.Today != "2026-11-10" || timing.Timezone != "America/Los_Angeles" || timing.Phase != PhasePreparing {
		t.Fatalf("timing = %+v", timing)
	}
	if timing.PreparationStartsOn == nil || *timing.PreparationStartsOn != "2026-10-28" {
		t.Fatalf("preparation start = %v, want 2026-10-28", timing.PreparationStartsOn)
	}
}

func TestPreparationCrossesTheYear(t *testing.T) {
	starts, ends := mustDate(t, "2026-12-31"), mustDate(t, "2027-01-02")
	prep, ok := PreparationStartsOn(starts, lead(10))
	if !ok || prep.String() != "2026-12-21" {
		t.Fatalf("preparation start = %s (%v), want 2026-12-21", prep, ok)
	}
	if got := Phase(mustDate(t, "2027-01-01"), starts, ends, lead(10)); got != PhaseLive {
		t.Fatalf("2027-01-01 phase = %s, want live", got)
	}
	if got := Phase(mustDate(t, "2026-12-21"), starts, ends, lead(10)); got != PhasePreparing {
		t.Fatalf("2026-12-21 phase = %s, want preparing", got)
	}
	if got := Phase(mustDate(t, "2026-12-20"), starts, ends, lead(10)); got != PhaseBeforePreparation {
		t.Fatalf("2026-12-20 phase = %s, want before_preparation", got)
	}
	if got := Phase(mustDate(t, "2027-01-03"), starts, ends, lead(10)); got != PhaseEnded {
		t.Fatalf("2027-01-03 phase = %s, want ended", got)
	}
}

// America/New_York leaves daylight saving time on 2026-11-01. Seven days
// before 11-05 is 10-29 on the calendar (spec Edge Cases).
func TestPreparationCountsCalendarDaysAcrossDaylightSaving(t *testing.T) {
	prep, ok := PreparationStartsOn(mustDate(t, "2026-11-05"), lead(7))
	if !ok || prep.String() != "2026-10-29" {
		t.Fatalf("New York preparation start = %s (%v), want 2026-10-29", prep, ok)
	}
}

// The fall-back case above does not catch hour arithmetic on its own: going
// back over a 25-hour day from local midnight lands at 01:00 of the right
// date. Going back over the 23-hour day New York has on 2027-03-14 lands at
// 23:00 of the day before, so this is the case that fails an implementation
// computing "local midnight minus n*24h" (plan.md mutation M2).
func TestPreparationCountsCalendarDaysAcrossSpringForward(t *testing.T) {
	prep, ok := PreparationStartsOn(mustDate(t, "2027-03-16"), lead(7))
	if !ok || prep.String() != "2027-03-09" {
		t.Fatalf("New York preparation start = %s (%v), want 2027-03-09", prep, ok)
	}
	ny := mustLocation(t, "America/New_York")
	byHours := time.Date(2027, 3, 16, 0, 0, 0, 0, ny).Add(-7 * 24 * time.Hour)
	if byHours.Day() == 9 {
		t.Fatal("hour arithmetic gives the calendar answer here too; this case stopped testing anything")
	}
}

func TestUnsetLeadIsNotZero(t *testing.T) {
	starts, ends := mustDate(t, "2026-11-11"), mustDate(t, "2026-11-12")
	if _, ok := PreparationStartsOn(starts, nil); ok {
		t.Fatal("unset lead produced a preparation start")
	}
	if got := Phase(mustDate(t, "2026-11-01"), starts, ends, nil); got != PhaseBeforeStartUnknownLead {
		t.Fatalf("unset lead before start = %s, want before_start_unknown_lead", got)
	}
	// Zero means "no preparation": live on the start day, not preparing the
	// day before.
	if got := Phase(starts, starts, ends, lead(0)); got != PhaseLive {
		t.Fatalf("lead 0 on start day = %s, want live", got)
	}
	if got := Phase(mustDate(t, "2026-11-10"), starts, ends, lead(0)); got != PhaseBeforePreparation {
		t.Fatalf("lead 0 the day before = %s, want before_preparation", got)
	}
	timing, err := ComputeTiming(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		NodeContent{StartsOn: "2026-11-11", EndsOn: "2026-11-12", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	if timing.PreparationStartsOn != nil {
		t.Fatalf("unset lead reported preparation start %s", *timing.PreparationStartsOn)
	}
}

func TestParseDateIsStrict(t *testing.T) {
	for _, bad := range []string{"", "2026-2-3", "2026-02-30", "2026/02/03", "2026-02-03T00:00:00Z", " 2026-02-03"} {
		if _, ok := ParseDate(bad); ok {
			t.Errorf("ParseDate(%q) accepted", bad)
		}
	}
	if d := mustDate(t, "2028-02-29"); d.AddDays(1).String() != "2028-03-01" {
		t.Fatalf("leap day + 1 = %s", d.AddDays(1))
	}
	if n := mustDate(t, "2026-12-31").DaysUntil(mustDate(t, "2027-01-02")); n != 2 {
		t.Fatalf("days until = %d, want 2", n)
	}
}
