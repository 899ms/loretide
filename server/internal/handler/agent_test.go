package handler

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestWorkspaceAlwaysRedactSecrets(t *testing.T) {
	tests := []struct {
		name     string
		settings []byte
		want     bool
	}{
		{"nil settings", nil, false},
		{"empty settings", []byte(`{}`), false},
		{"false", []byte(`{"always_redact_env": false}`), false},
		{"true", []byte(`{"always_redact_env": true}`), true},
		{"invalid json", []byte(`not json`), false},
		{"other fields only", []byte(`{"theme": "dark"}`), false},
		{"true among other fields", []byte(`{"theme": "dark", "always_redact_env": true}`), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workspaceAlwaysRedactSecrets(tt.settings); got != tt.want {
				t.Errorf("workspaceAlwaysRedactSecrets(%q) = %v, want %v", tt.settings, got, tt.want)
			}
		})
	}
}

// rawJSONResponse decodes the raw map so we can assert the literal
// JSON shape — `custom_env` MUST be absent from the wire output, not
// merely empty, otherwise a future caller decoding into a wider struct
// could still see masked or partial values.
func rawJSONResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return out
}

// TestMergeAgentEnv_PureFunction exercises the diff/sentinel logic
// without the DB round-trip — keeps the contract front-and-centre in
// case someone refactors the handler later.
func TestMergeAgentEnv_PureFunction(t *testing.T) {
	cases := []struct {
		name     string
		existing map[string]string
		request  map[string]string
		want     map[string]string
		audit    envAudit
	}{
		{
			name:     "preserve sentinel",
			existing: map[string]string{"A": "real"},
			request:  map[string]string{"A": "****"},
			want:     map[string]string{"A": "real"},
			audit:    envAudit{preserved: []string{"A"}},
		},
		{
			name:     "drop sentinel for missing key",
			existing: map[string]string{},
			request:  map[string]string{"A": "****"},
			want:     map[string]string{},
			audit:    envAudit{},
		},
		{
			name:     "add new key",
			existing: map[string]string{},
			request:  map[string]string{"B": "v"},
			want:     map[string]string{"B": "v"},
			audit:    envAudit{added: []string{"B"}},
		},
		{
			name:     "change existing value",
			existing: map[string]string{"B": "old"},
			request:  map[string]string{"B": "new"},
			want:     map[string]string{"B": "new"},
			audit:    envAudit{changed: []string{"B"}},
		},
		{
			name:     "remove key absent from request",
			existing: map[string]string{"B": "v"},
			request:  map[string]string{},
			want:     map[string]string{},
			audit:    envAudit{removed: []string{"B"}},
		},
		{
			name:     "noop when value unchanged",
			existing: map[string]string{"B": "same"},
			request:  map[string]string{"B": "same"},
			want:     map[string]string{"B": "same"},
			audit:    envAudit{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, audit := mergeAgentEnv(tc.existing, tc.request)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("merged map: got %v, want %v", got, tc.want)
			}
			if !reflect.DeepEqual(audit, tc.audit) {
				t.Errorf("audit: got %+v, want %+v", audit, tc.audit)
			}
		})
	}
}

// Defence-in-depth: spot-check that the package compiles a small
// fmt.Sprintf so accidental imports stay tidy.
var _ = fmt.Sprintf
