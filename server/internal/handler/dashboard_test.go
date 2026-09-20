package handler

import (
	"encoding/json"
	"testing"
	"time"
)

// dashboardFixtureTZ is the zone the day-boundary fixtures in this file are
// built in, and the zone their requests pin with `?tz=`. Both sides read this
// one constant because they have to agree: every `days=N` endpoint opens its
// window at start-of-day in the VIEWER's zone (resolveViewingTZ: `?tz=`, else
// the user's stored user.timezone, else UTC), so a fixture anchored in one
// zone and a window opened in another sit an offset apart. Without the
// `?tz=` these requests would inherit whatever user.timezone holds — NULL in
// the handler fixture, hence UTC.
//
// The zone is deliberately EAST of UTC, and that is what makes the pin
// load-bearing rather than decorative. A fixture built in UTC survives losing
// its `?tz=`: the fallback is UTC too, so the window opens where the fixture
// already sits and nothing notices. Built in Asia/Tokyo the run finishes at
// 15:10 UTC the previous day, so a request that falls back to UTC opens its
// window after the run ended and the assertions go red — which is the whole
// point of pinning it. Verified by mutation: dropping the parameter, and
// pinning it to UTC, both fail.
const dashboardFixtureTZ = "Asia/Tokyo"

// dashboardFixtureTZParam is dashboardFixtureTZ as a query fragment, so a
// request cannot pin a zone the fixture was not built in.
const dashboardFixtureTZParam = "tz=" + dashboardFixtureTZ

// dashboardFixtureLoc resolves dashboardFixtureTZ.
func dashboardFixtureLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(dashboardFixtureTZ)
	if err != nil {
		t.Fatalf("load fixture timezone %q: %v", dashboardFixtureTZ, err)
	}
	return loc
}

// runFinishedToday returns the started_at / completed_at of a ten-minute run
// placed in the first ten minutes of the day `now` falls in, read in loc.
//
// Every endpoint these fixtures exercise filters on `completed_at >= @since`
// — the END of the run, not its start — and for the per-agent rollups @since
// is start of today in the viewer's zone (parseExactSinceParamInTZ at
// days=1). Timestamps built as `now - 30m` therefore fall out of that window
// for the first twenty minutes after midnight: the run finished yesterday,
// the window opens today, and the assertions read the empty result as nothing
// having happened. A handler job that ran at 00:13 UTC failed on exactly
// that; the next one at 00:25 passed with nothing changed but the clock. The
// date-bucketed halves ride out those twenty minutes on the extra day
// parseSinceParamInTZ hands them, which is why only their per-agent siblings
// went red. Anchoring the run on the boundary itself takes the time of day
// out of the fixture rather than special-casing the twenty minutes it bites.
//
// Start-of-day is built the way sinceFromDays builds the cutoff, so fixture
// and window agree instant for instant. In the first ten minutes after
// midnight the run ends slightly in the future, which none of these queries
// mind: they bound completed_at from below only, and the token rollup keys
// off task_usage.created_at rather than the queue row.
func runFinishedToday(now time.Time, loc *time.Location) (started, completed time.Time) {
	local := now.In(loc)
	started = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return started, started.Add(10 * time.Minute)
}

// pinDayWindowClock freezes the clock the request-side cutoff reads and hands
// back the same instant for the fixture, so both sides describe one moment.
//
// They read the wall clock at two different points otherwise — the fixture when
// it is built, the cutoff when the handler runs, with inserts and a rollup in
// between — and a suite that crosses midnight in that gap writes its run into
// one day and then asks for the next day's window. The gap is milliseconds, so
// it is rare and permanent: nothing about the assertions says which day they
// meant.
func pinDayWindowClock(t *testing.T, at time.Time) time.Time {
	t.Helper()
	prev := dayWindowNow
	dayWindowNow = func() time.Time { return at }
	t.Cleanup(func() { dayWindowNow = prev })
	return at
}

// TestDashboardFixtureRunLandsInsideTheWindow pins what runFinishedToday
// promises the two DB fixtures that call it: at any hour, in any zone, the
// run it places is inside the days=1 window the endpoints open. The window is
// taken from sinceFromDays — the production cutoff itself — rather than from
// a second copy of the helper's arithmetic.
//
// The midnight rows are the point of the table. A fixture built off the wall
// clock is outside that window for the first twenty minutes of the day and
// inside it for the other 23h40m, so only a synthetic clock can hold it to
// account: the suite would otherwise have to run at midnight to see the
// failure, which is how this reached CI in the first place.
func TestDashboardFixtureRunLandsInsideTheWindow(t *testing.T) {
	// Half-hour and negative offsets, so a helper that anchored on UTC
	// midnight while the request asked for another zone cannot pass.
	for _, tz := range []string{dashboardFixtureTZ, "Asia/Kolkata", "America/Los_Angeles"} {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			t.Fatalf("load %s: %v", tz, err)
		}
		for _, clock := range []string{
			"2026-03-01 00:00:00",
			"2026-03-01 00:05:00",
			"2026-03-01 00:19:59",
			"2026-03-01 12:00:00",
			"2026-03-01 23:59:59",
		} {
			t.Run(tz+" "+clock, func(t *testing.T) {
				now, err := time.ParseInLocation("2006-01-02 15:04:05", clock, loc)
				if err != nil {
					t.Fatalf("parse %s: %v", clock, err)
				}
				started, completed := runFinishedToday(now, loc)

				// parseExactSinceParamInTZ trims a day off sinceFromDays, so
				// the days=1 cutoff these endpoints use is start of today.
				since := sinceFromDays(now, 0, loc)
				tomorrow := since.AddDate(0, 0, 1)

				if got := completed.Sub(started); got != 10*time.Minute {
					t.Errorf("run lasted %s, want 10m — the >=600s the run-time assertion reads", got)
				}
				if completed.Before(since) {
					t.Errorf("completed_at %s precedes the days=1 cutoff %s: `completed_at >= @since` drops the fixture", completed, since)
				}
				if !completed.Before(tomorrow) {
					t.Errorf("completed_at %s is past the day that opened at %s", completed, since)
				}
				if started.Before(since) {
					t.Errorf("started_at %s precedes the days=1 cutoff %s", started, since)
				}
			})
		}
	}
}

// TestDashboardFailureWireContractKeepsEmptyReason pins the success bucket's
// wire form. The client's zod schema defaults a missing `failure_reason` to
// "" — the succeeded bucket — which is only safe while the server always
// emits the field. Adding `omitempty` to the struct tag would strip it from
// exactly the success rows and silently turn every window into a 100% error
// rate, so that regression is caught here rather than in a dashboard.
func TestDashboardFailureWireContractKeepsEmptyReason(t *testing.T) {
	// Each case decodes into its OWN map. json.Unmarshal merges into a
	// non-nil map rather than resetting it, so sharing one across cases would
	// leave the first payload's failure_reason in place and let a later
	// omitempty regression pass unnoticed — the exact failure this test
	// exists to catch.
	for _, tc := range []struct {
		name string
		row  any
	}{
		{"daily", DashboardFailureDailyResponse{Date: "2026-05-19", TaskCount: 3}},
		{"by-agent", DashboardFailureByAgentResponse{AgentID: "a", TaskCount: 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(tc.row)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			decoded := map[string]any{}
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if _, present := decoded["failure_reason"]; !present {
				t.Errorf("succeeded rows must serialize an explicit empty failure_reason, got %s", body)
			}
		})
	}
}
