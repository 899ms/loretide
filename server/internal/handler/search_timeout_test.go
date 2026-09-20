package handler

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"testing"
	"time"
)

func TestIsSearchStatementTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"57014 pgx error", &pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"}, true},
		{"57014 wrapped", errors.Join(errors.New("outer"), &pgconn.PgError{Code: "57014"}), true},
		{"different pg code", &pgconn.PgError{Code: "42P01"}, false},
		{"plain error", errors.New("boom"), false},
		{"context canceled", context.Canceled, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSearchStatementTimeout(tc.err); got != tc.want {
				t.Errorf("isSearchStatementTimeout(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestParseSearchWorkMemMB(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   int
		wantOK bool
	}{
		{name: "unset uses default", want: defaultSearchWorkMemMB, wantOK: true},
		{name: "disable local override", raw: "0", want: 0, wantOK: true},
		{name: "lower cap", raw: " 16 ", want: 16, wantOK: true},
		{name: "default explicitly", raw: "64", want: 64, wantOK: true},
		{name: "negative rejected", raw: "-1", want: defaultSearchWorkMemMB},
		{name: "higher cap rejected", raw: "65", want: defaultSearchWorkMemMB},
		{name: "unit suffix rejected", raw: "16MB", want: defaultSearchWorkMemMB},
		{name: "invalid rejected", raw: "large", want: defaultSearchWorkMemMB},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseSearchWorkMemMB(tt.raw)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("parseSearchWorkMemMB(%q) = (%d, %t), want (%d, %t)", tt.raw, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// setSearchStatementTimeoutForTest is a package-private hook used only
// by the live-Postgres timeout test above. Kept out of the public
// surface to prevent handlers from accidentally raising the cap.
func setSearchStatementTimeoutForTest(t *testing.T, v time.Duration) {
	t.Helper()
	searchStatementTimeoutOverride = v
}
