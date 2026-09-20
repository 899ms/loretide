package handler

import (
	"context"
	"strings"
	"testing"
)

// TestThinkingLevelRejectionCopy pins WHY a thinking_level was refused, not
// just that it was. A runtime with no reasoning control must not be described
// as receiving an unrecognised value: "high" is a fine effort token, and the
// old shared sentence sent users looking for a spelling that cannot exist
// (MUL-5770).
//
// Copilot is the standing example: it discovers over ACP but executes through
// its own CLI, so no live ACP session exists to carry an effort. Hermes used
// to play this role and no longer can — jcode runs under that provider and
// applies an advertised effort, so the provider-level gate is open for it.
func TestThinkingLevelRejectionCopy(t *testing.T) {
	t.Parallel()

	t.Run("runtime without reasoning control names the capability gap", func(t *testing.T) {
		msg := thinkingLevelRejection("copilot", "high")
		if !strings.Contains(msg, "does not support a per-agent reasoning effort") {
			t.Errorf("expected a capability explanation, got %q", msg)
		}
		if strings.Contains(msg, "not a recognised value") {
			t.Errorf("capability gap must not read as a bad token, got %q", msg)
		}
		if !strings.Contains(msg, `"copilot"`) {
			t.Errorf("expected the runtime named in %q", msg)
		}
	})

	t.Run("runtime with reasoning control names the value", func(t *testing.T) {
		msg := thinkingLevelRejection("claude", "supersonic")
		if !strings.Contains(msg, "not a recognised value") {
			t.Errorf("expected a value-level explanation, got %q", msg)
		}
		if !strings.Contains(msg, `"supersonic"`) {
			t.Errorf("expected the rejected token echoed in %q", msg)
		}
	})

	t.Run("carry-over path always offers the clear escape hatch", func(t *testing.T) {
		for _, provider := range []string{"copilot", "claude"} {
			msg := existingThinkingLevelRejection(provider, "xhigh")
			if !strings.Contains(msg, `thinking_level=""`) {
				t.Errorf("provider %q: expected the clear instruction, got %q", provider, msg)
			}
		}
		if msg := existingThinkingLevelRejection("copilot", "xhigh"); !strings.Contains(msg, "does not support a per-agent reasoning effort") {
			t.Errorf("expected the capability explanation on the carry-over path, got %q", msg)
		}
	})
}

// TestAcpThinkingDecision is the fix for the regression the hermes
// change introduced: `hermes` covers jcode (advertises an effort) and Hermes
// Agent (advertises none), and the provider name cannot tell them apart. The
// discovered catalog can, so the capability 400 survives for the binary that
// genuinely has no reasoning dial.
func TestAcpThinkingDecision(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	runtimeID := parseUUID("11111111-1111-1111-1111-111111111111")

	withCatalog := func(t *testing.T, models []ModelEntry) *Handler {
		t.Helper()
		cache := NewInMemoryModelCatalogCache()
		if err := cache.Put(ctx, uuidToString(runtimeID), models, nil, true); err != nil {
			t.Fatalf("seed catalog: %v", err)
		}
		return &Handler{ModelCatalogCache: cache}
	}

	withEffort := []ModelEntry{{
		ID:       "gpt-5.6-sol",
		Thinking: &ModelThinking{SupportedLevels: []ThinkingLevel{{Value: "high", Label: "High"}}},
	}}
	withoutEffort := []ModelEntry{{ID: "hermes-4"}}

	t.Run("jcode-shaped catalog keeps the level accepted", func(t *testing.T) {
		if got := withCatalog(t, withEffort).acpThinkingDecision(ctx, "hermes", runtimeID); got != acpEffortPresent {
			t.Errorf("decision = %v, want acpEffortPresent", got)
		}
	})

	t.Run("Hermes-Agent-shaped catalog restores the capability answer", func(t *testing.T) {
		if got := withCatalog(t, withoutEffort).acpThinkingDecision(ctx, "hermes", runtimeID); got != acpEffortAbsent {
			t.Errorf("decision = %v, want acpEffortAbsent", got)
		}
	})

	// Undiscovered is its own answer for an ambiguous provider — not "supported".
	// The catalog is only written once a client asks for a model list, so this
	// state persists indefinitely for CLI-only callers.
	t.Run("no cache is unknown for hermes", func(t *testing.T) {
		if got := (&Handler{}).acpThinkingDecision(ctx, "hermes", runtimeID); got != acpEffortUnknown {
			t.Errorf("decision = %v, want acpEffortUnknown", got)
		}
	})

	t.Run("empty catalog is unknown for hermes", func(t *testing.T) {
		if got := withCatalog(t, nil).acpThinkingDecision(ctx, "hermes", runtimeID); got != acpEffortUnknown {
			t.Errorf("decision = %v, want acpEffortUnknown", got)
		}
	})

	// reasonix names exactly one binary, and that binary supports an effort, so
	// an undiscovered reasonix runtime is allowed rather than blocked before its
	// first discovery. Tightening hermes must not tighten this.
	t.Run("no cache still allows reasonix", func(t *testing.T) {
		if got := (&Handler{}).acpThinkingDecision(ctx, "reasonix", runtimeID); got != acpEffortPresent {
			t.Errorf("decision = %v, want acpEffortPresent — reasonix is unambiguous", got)
		}
	})

	// Providers outside the ACP-catalog set are answered by name alone; this
	// check must not start second-guessing them from a catalog.
	t.Run("non-ACP provider is not consulted", func(t *testing.T) {
		if got := withCatalog(t, withoutEffort).acpThinkingDecision(ctx, "claude", runtimeID); got != acpEffortPresent {
			t.Errorf("decision = %v, want acpEffortPresent — claude is decided by provider", got)
		}
	})
}
