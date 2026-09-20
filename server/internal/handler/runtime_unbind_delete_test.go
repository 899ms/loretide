package handler

import (
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"testing"
)

type recordingRuntimeGoneNotifier struct {
	runtimeIDs []string
}

func (n *recordingRuntimeGoneNotifier) NotifyRuntimeGone(runtimeID string) {
	n.runtimeIDs = append(n.runtimeIDs, runtimeID)
}

// parseExpectedActiveAgentIDs is the cascade endpoint's input validator.
// Empty list is a valid plan ("no active agents" — cascade just deletes the
// runtime); malformed UUIDs must surface as 400 so a bug in the front-end
// can't silently dilute the plan check.
func TestParseExpectedActiveAgentIDs(t *testing.T) {
	t.Run("empty list returns empty set, ok", func(t *testing.T) {
		got, ok := parseExpectedActiveAgentIDs(nil)
		if !ok {
			t.Fatalf("expected ok for nil input")
		}
		if len(got) != 0 {
			t.Fatalf("expected empty set, got %d entries", len(got))
		}
	})

	t.Run("valid uuids are accepted and deduplicated by set semantics", func(t *testing.T) {
		ids := []string{
			"11111111-1111-1111-1111-111111111111",
			"22222222-2222-2222-2222-222222222222",
			"11111111-1111-1111-1111-111111111111", // dup is intentional
		}
		got, ok := parseExpectedActiveAgentIDs(ids)
		if !ok {
			t.Fatalf("expected ok for valid uuid list")
		}
		if len(got) != 2 {
			t.Fatalf("expected dedup set of 2, got %d", len(got))
		}
		for _, want := range []string{
			"11111111-1111-1111-1111-111111111111",
			"22222222-2222-2222-2222-222222222222",
		} {
			if _, ok := got[want]; !ok {
				t.Fatalf("expected %s in set", want)
			}
		}
	})

	t.Run("any malformed entry fails the whole list", func(t *testing.T) {
		ids := []string{
			"11111111-1111-1111-1111-111111111111",
			"not-a-uuid",
		}
		_, ok := parseExpectedActiveAgentIDs(ids)
		if ok {
			t.Fatal("expected !ok for list containing malformed uuid")
		}
	})
}

// activeAgentSetMatches drives the runtime_delete_plan_changed branch: it
// must report mismatch for any divergence — extra agent, missing agent, or
// substituted agent — and accept order-insensitive set equality.
func TestActiveAgentSetMatches(t *testing.T) {
	mkAgent := func(id string) db.Agent {
		u, err := uuidFromString(id)
		if err != nil {
			t.Fatalf("uuidFromString: %v", err)
		}
		return db.Agent{ID: u}
	}
	a1 := mkAgent("11111111-1111-1111-1111-111111111111")
	a2 := mkAgent("22222222-2222-2222-2222-222222222222")
	a3 := mkAgent("33333333-3333-3333-3333-333333333333")

	t.Run("equal sets match regardless of order", func(t *testing.T) {
		expected := map[string]struct{}{
			"11111111-1111-1111-1111-111111111111": {},
			"22222222-2222-2222-2222-222222222222": {},
		}
		if !activeAgentSetMatches([]db.Agent{a2, a1}, expected) {
			t.Fatal("expected match for set-equal inputs")
		}
	})

	t.Run("missing agent is a mismatch", func(t *testing.T) {
		expected := map[string]struct{}{
			"11111111-1111-1111-1111-111111111111": {},
			"22222222-2222-2222-2222-222222222222": {},
		}
		if activeAgentSetMatches([]db.Agent{a1}, expected) {
			t.Fatal("expected mismatch when an agent disappeared")
		}
	})

	t.Run("extra agent is a mismatch", func(t *testing.T) {
		expected := map[string]struct{}{
			"11111111-1111-1111-1111-111111111111": {},
		}
		if activeAgentSetMatches([]db.Agent{a1, a2}, expected) {
			t.Fatal("expected mismatch when a new agent appeared")
		}
	})

	t.Run("substituted agent is a mismatch", func(t *testing.T) {
		expected := map[string]struct{}{
			"11111111-1111-1111-1111-111111111111": {},
			"22222222-2222-2222-2222-222222222222": {},
		}
		if activeAgentSetMatches([]db.Agent{a1, a3}, expected) {
			t.Fatal("expected mismatch when one agent was swapped for another")
		}
	})

	t.Run("both empty matches", func(t *testing.T) {
		if !activeAgentSetMatches(nil, map[string]struct{}{}) {
			t.Fatal("expected empty/empty to match")
		}
	})
}

func uuidFromString(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}
