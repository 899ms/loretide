package handler

import (
	"slices"
	"testing"
	"time"
)

func TestTaskMessageCreatedAtRejectsImplausibleClockSkew(t *testing.T) {
	t.Parallel()

	serverNow := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	zero := time.Time{}
	insidePast := serverNow.Add(-maxTaskMessageClockSkew)
	insideFuture := serverNow.Add(maxTaskMessageClockSkew)
	outsidePast := insidePast.Add(-time.Nanosecond)
	outsideFuture := insideFuture.Add(time.Nanosecond)

	tests := []struct {
		name string
		at   *time.Time
		want string
	}{
		{name: "missing"},
		{name: "zero", at: &zero},
		{name: "past boundary", at: &insidePast, want: insidePast.Format(time.RFC3339Nano)},
		{name: "future boundary", at: &insideFuture, want: insideFuture.Format(time.RFC3339Nano)},
		{name: "too far in the past", at: &outsidePast},
		{name: "too far in the future", at: &outsideFuture},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := taskMessageCreatedAt(tt.at, serverNow); got != tt.want {
				t.Fatalf("taskMessageCreatedAt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTaskMessageCreatedAtsFallsBackWholeBatch(t *testing.T) {
	t.Parallel()

	serverNow := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	valid := serverNow.Add(-time.Second)
	invalid := serverNow.Add(-maxTaskMessageClockSkew - time.Nanosecond)

	got := taskMessageCreatedAts([]TaskMessageRequest{
		{CreatedAt: &valid},
		{CreatedAt: &invalid},
	}, serverNow)
	if want := []string{"", ""}; !slices.Equal(got, want) {
		t.Fatalf("taskMessageCreatedAts() = %q, want whole-batch fallback %q", got, want)
	}

	got = taskMessageCreatedAts([]TaskMessageRequest{
		{CreatedAt: &valid},
		{CreatedAt: &serverNow},
	}, serverNow)
	if want := []string{valid.Format(time.RFC3339Nano), serverNow.Format(time.RFC3339Nano)}; !slices.Equal(got, want) {
		t.Fatalf("taskMessageCreatedAts() = %q, want %q", got, want)
	}
}
