package handler

import (
	"encoding/json"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeAgent(t *testing.T, w *httptest.ResponseRecorder) AgentResponse {
	t.Helper()
	var resp AgentResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode agent response: %v (body %s)", err, w.Body.String())
	}
	return resp
}

// TestComposeMikaInstructions covers the layering contract: the product half
// always leads, workspace notes are labelled with their provenance, and an
// empty second layer adds nothing — including the section heading that
// describes it.
func TestComposeMikaInstructions(t *testing.T) {
	system := service.MikaSystemInstructions(service.MikaDefaultName)

	if got := service.ComposeMikaInstructions(service.MikaDefaultName, ""); got != system {
		t.Fatal("an empty workspace layer must compose to exactly the system layer")
	}
	if got := service.ComposeMikaInstructions(service.MikaDefaultName, "   \n  "); got != system {
		t.Fatal("a blank workspace layer must compose to exactly the system layer")
	}
	// Without notes the prompt must not end by announcing a section that has
	// nothing under it.
	if strings.Contains(system, "Workspace notes below add") {
		t.Fatalf("the notes rule must not appear when there are no notes:\n%s", system)
	}

	composed := service.ComposeMikaInstructions(service.MikaDefaultName, "Our main repo is acme/platform.")
	if !strings.HasPrefix(composed, system) {
		t.Fatal("the system layer must lead the composed prompt")
	}
	for _, want := range []string{
		"## Workspace notes",
		"Workspace notes below add",
		"Added by this workspace's admins",
		"Our main repo is acme/platform.",
	} {
		if !strings.Contains(composed, want) {
			t.Fatalf("composed prompt missing %q:\n%s", want, composed)
		}
	}
}

// The runtime brief announces "**You are: <name>**" from the agent row, so a
// hardcoded "You are Mika" would contradict it the moment an owner renames the
// agent.
func TestMikaSystemInstructionsUsesTheCurrentDisplayName(t *testing.T) {
	renamed := service.MikaSystemInstructions("Jarvis")
	if !strings.HasPrefix(renamed, "You are Jarvis,") {
		t.Fatalf("prompt should open as the current name:\n%s", renamed[:120])
	}
	if strings.Contains(renamed, "{{AGENT_NAME}}") {
		t.Fatal("the name placeholder must be substituted")
	}
	// The product identity is still stated, just not as the display name.
	if !strings.Contains(renamed, "built-in system agent (Mika)") {
		t.Fatal("prompt should still identify itself as Multica's built-in agent")
	}

	if blank := service.MikaSystemInstructions("   "); !strings.HasPrefix(blank, "You are Mika,") {
		t.Fatalf("a blank name should fall back to the default:\n%s", blank[:120])
	}
}

// TestSystemInstructionsFor_OnlySystemAgents guards the blast radius: an
// ordinary agent's payload must be byte-identical to before this feature.
func TestSystemInstructionsFor_OnlySystemAgents(t *testing.T) {
	mika := db.Agent{SystemKey: pgtype.Text{String: service.MikaSystemKey, Valid: true}}
	if systemInstructionsFor(mika) == "" {
		t.Fatal("Mika should expose the product prompt")
	}
	for _, ordinary := range []db.Agent{
		{},
		{SystemKey: pgtype.Text{String: "", Valid: true}},
		{SystemKey: pgtype.Text{String: "agent_builder:abc", Valid: true}},
	} {
		if got := systemInstructionsFor(ordinary); got != "" {
			t.Fatalf("non-Mika agent must expose no system instructions, got %q", got)
		}
	}
}
