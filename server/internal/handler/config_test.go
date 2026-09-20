package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestURLHostEqualsCanonicalizesCommonHostForms(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "full URL", raw: "https://api.multica.ai", want: true},
		{name: "bare host", raw: "api.multica.ai", want: true},
		{name: "host port", raw: "api.multica.ai:8080", want: true},
		{name: "trailing dot", raw: "https://api.multica.ai.", want: true},
		{name: "different host", raw: "https://evil.example", want: false},
		{name: "empty", raw: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := urlHostEquals(tt.raw, "api.multica.ai"); got != tt.want {
				t.Fatalf("urlHostEquals(%q): want %v, got %v", tt.raw, tt.want, got)
			}
		})
	}
}

func TestGetConfigExposesFrontendFeatureFlags(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	h.GetConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetConfig default flags: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var cfg AppConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode default config: %v", err)
	}
	if cfg.FeatureFlags["composio_mcp_apps"] {
		t.Fatalf("composio_mcp_apps: want false by default, got true")
	}
	if cfg.FeatureFlags["billing_workspace_subscriptions"] {
		t.Fatalf("billing_workspace_subscriptions: want false by default, got true")
	}
	if cfg.FeatureFlags["plugins_v1"] {
		t.Fatalf("plugins_v1: want false by default, got true")
	}
	for _, retired := range []string{"private_plugins_v1", "remote_mcp_plugins_v1"} {
		if _, published := cfg.FeatureFlags[retired]; published {
			t.Fatalf("retired Plugin sub-flag %q must not be published", retired)
		}
	}
	if !cfg.FeatureFlags["agents_skill_toggles"] {
		t.Fatalf("agents_skill_toggles: want true for installed v0.4.0 clients, got false")
	}
	if !cfg.FeatureFlags["settings_resource_labels"] {
		t.Fatalf("settings_resource_labels: want true for installed clients, got false")
	}
	if !cfg.FeatureFlags["agents_agent_builder"] {
		t.Fatalf("agents_agent_builder: want true for installed clients, got false")
	}
	// MUL-5345: hang stack capture is gone from this build, but v0.4.13–v0.4.18 are
	// installed and still hold a debugger channel open on every renderer whenever
	// this key arrives as `true`. Those clients are fail-closed on absence, so NOT
	// publishing the key is what disarms them — re-adding it would put a flag flip
	// back within reach of a fleet that can no longer produce a usable stack.
	if _, published := cfg.FeatureFlags["desktop_hang_stack_capture"]; published {
		t.Fatalf("desktop_hang_stack_capture: must stay unpublished so installed clients keep their debugger channels closed")
	}
	// Deliberately unpublished: pre-v0.4.33 clients gate their "New status"
	// button on this key and fail closed, which is how a client that predates
	// the v0.4.31 rendering fixes is kept from creating one.
	if _, published := cfg.FeatureFlags["custom_issue_statuses"]; published {
		t.Fatalf("custom_issue_statuses: want unpublished, got %v", cfg.FeatureFlags["custom_issue_statuses"])
	}

	withComposioMCPAppsFlag(t, h, true)
	w = httptest.NewRecorder()
	h.GetConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetConfig enabled flags: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode enabled config: %v", err)
	}
	if !cfg.FeatureFlags["composio_mcp_apps"] {
		t.Fatalf("composio_mcp_apps: want true with flag enabled, got false")
	}
}

func TestGetConfigExposesEnabledPluginsV1Flag(t *testing.T) {
	h := &Handler{}
	withPluginsV1Flag(t, h, true)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	w := httptest.NewRecorder()
	h.GetConfig(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetConfig enabled plugins_v1: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var cfg AppConfig
	if err := json.Unmarshal(w.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode enabled config: %v", err)
	}
	if !cfg.FeatureFlags["plugins_v1"] {
		t.Fatal("plugins_v1: want true with flag enabled, got false")
	}
}
