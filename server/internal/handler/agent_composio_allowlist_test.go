package handler

import (
	"testing"
)

// TestNormaliseComposioToolkitAllowlist_PureFunction exercises the canonical
// normalisation so future refactors don't silently change the persisted
// form (and break the dispatch path's flat slug compare).
func TestNormaliseComposioToolkitAllowlist_PureFunction(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil-in nil-out", nil, nil},
		{"empty-in empty-out", []string{}, []string{}},
		{"trim", []string{"  notion  "}, []string{"notion"}},
		{"lower", []string{"NOTION", "GitHub"}, []string{"notion", "github"}},
		{"dedupe", []string{"notion", "NOTION", "notion"}, []string{"notion"}},
		{"drop empty", []string{"", "   ", "notion"}, []string{"notion"}},
		{"preserve order of first-seen", []string{"notion", "github", "notion"}, []string{"notion", "github"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normaliseComposioToolkitAllowlist(tc.in)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("got %v; want nil", got)
				}
				return
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v; want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d]=%q; want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
