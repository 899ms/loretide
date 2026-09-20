package workspacecore

import (
	"testing"
	"time"
)

// Three answers, and the third one is the point.

func at(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return &parsed
}

func TestObservationDueHasExactlyThreeAnswers(t *testing.T) {
	now := *at("2026-09-20T00:00:00Z")

	for _, tc := range []struct {
		name        string
		publishedAt *time.Time
		days        int64
		hasDays     bool
		want        Due
	}{
		{"the window has elapsed", at("2026-09-01T00:00:00Z"), 14, true, DuePassed},
		{"the window is exactly up", at("2026-09-06T00:00:00Z"), 14, true, DuePassed},
		{"too early to expect numbers", at("2026-09-19T00:00:00Z"), 14, true, DueNotYet},
		// 0 days is a real window: look the same day.
		{"a same-day window, published today", at("2026-09-20T00:00:00Z"), 0, true, DuePassed},
		// No window set anywhere. NOT DueNotYet.
		{"the brand set no window", at("2026-09-01T00:00:00Z"), 0, false, DueUnknown},
		// 025 allows a publication record with no publication time: somebody
		// writing down a piece that went out last month may not know the hour.
		{"no publication time", nil, 14, true, DueUnknown},
		{"neither", nil, 0, false, DueUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ObservationDue(tc.publishedAt, tc.days, tc.hasDays, now)
			if got != tc.want {
				t.Errorf("= %q, want %q", got, tc.want)
			}
		})
	}
}

// The mistake this whole three-valued type exists to prevent. If unknown were
// answered as not_yet, every record without a publication time would quietly
// leave the workbench - and those are exactly the ones somebody should look at.
func TestUnknownIsNotAQuieterNotYet(t *testing.T) {
	now := *at("2026-09-20T00:00:00Z")
	if got := ObservationDue(nil, 14, true, now); got == DueNotYet {
		t.Fatal("a record with no publication time answered not_yet; it must answer unknown")
	}
	if got := ObservationDue(at("2026-09-01T00:00:00Z"), 0, false, now); got == DueNotYet {
		t.Fatal("a brand with no window answered not_yet; it must answer unknown")
	}
}

func TestObservationDueForResolvesTheWindowItself(t *testing.T) {
	now := *at("2026-09-20T00:00:00Z")
	rules := DefaultRules()
	rules.Observation.Default = days(14)
	rules.Observation.ByChannel["douyin"] = 2

	published := at("2026-09-15T00:00:00Z")

	// Five days in: past Douyin's two, short of the brand's fourteen.
	if got := ObservationDueFor(rules, "douyin", published, now); got != DuePassed {
		t.Errorf("douyin = %q, want passed", got)
	}
	if got := ObservationDueFor(rules, "xiaohongshu", published, now); got != DueNotYet {
		t.Errorf("xiaohongshu = %q, want not_yet", got)
	}

	bare := DefaultRules()
	if got := ObservationDueFor(bare, "xiaohongshu", published, now); got != DueUnknown {
		t.Errorf("with nothing set = %q, want unknown", got)
	}
}
