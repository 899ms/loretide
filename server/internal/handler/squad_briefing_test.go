package handler

import (
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"testing"
)

// TestSquadOperatingProtocolScopesParentStatusOwnership is the guard for the
// MUL-5156 review finding: the briefing is injected on every leader path,
// including an @squad mention on an issue assigned to someone else. Status
// ownership must not ride along — a guest leader gets an explicit prohibition
// instead of the grant, so the model never has to infer the boundary.
func TestSquadOperatingProtocolScopesParentStatusOwnership(t *testing.T) {
	guest := squadOperatingProtocolFor(false)
	compactGuest := strings.Join(strings.Fields(guest), " ")

	for _, want := range []string{
		"Do NOT change this issue's status",
		"not assigned to your squad",
		"never run `multica issue status` on it",
	} {
		if !strings.Contains(compactGuest, want) {
			t.Errorf("expected guest-leader protocol to contain %q\n--- protocol ---\n%s", want, guest)
		}
	}
	// The grant must be entirely absent — not merely qualified.
	for _, forbidden := range []string{
		"Own the parent issue status",
		"multica issue status <issue-id> in_review",
	} {
		if strings.Contains(compactGuest, forbidden) {
			t.Errorf("guest-leader protocol must not contain status grant %q\n--- protocol ---\n%s", forbidden, guest)
		}
	}

	// Everything that is not the status responsibility is identical, so a
	// guest leader still coordinates, delegates, and records activity.
	owner := squadOperatingProtocolFor(true)
	for _, shared := range []string{
		"## Squad Operating Protocol",
		"Delegate by @mention",
		"Record your evaluation",
		"Stop after dispatching",
		"Never both for the same work.",
	} {
		if !strings.Contains(owner, shared) || !strings.Contains(guest, shared) {
			t.Errorf("expected %q in both protocol variants", shared)
		}
	}

	// Both variants must keep the protocol header. The daemon no longer
	// derives IsSquadLeader from it (MUL-5811 — it reads is_leader_task /
	// squad_id off the claim), but it is still the section title the leader
	// rules in the brief and the per-turn prompt refer to by name.
	if !strings.Contains(guest, "## Squad Operating Protocol") {
		t.Error("guest-leader protocol lost its section header")
	}
}

// TestSquadOperatingProtocolOwnsNoActionRule pins the protocol as the single
// statement of the no_action rule (MUL-6984). It used to be written four
// times — here, in the per-turn prompt, in the brief's workflow step 4, and in
// the brief's ## Output — and the four copies had already drifted: only some
// of them carried the MUL-6622 / GH #7487 escape hatch, which is what keeps a
// FAILED `squad activity` call from ending the turn in silence. The other
// three surfaces now point here, so this text has to carry the whole rule:
// the prohibition, its exact scope, and the failure fallback.
func TestSquadOperatingProtocolOwnsNoActionRule(t *testing.T) {
	for _, ownsParentStatus := range []bool{true, false} {
		protocol := squadOperatingProtocolFor(ownsParentStatus)
		compact := strings.Join(strings.Fields(protocol), " ")

		for _, want := range []string{
			// the rule and how it is recorded
			"multica squad activity <issue-id> <outcome> --reason",
			"record `no_action` and exit silently",
			// what "silently" forbids — MUL-2168 was a leader posting
			// "no reply needed. Exiting silently."
			"posting NO comment at all",
			"not one saying you are exiting",
			// MUL-6622 / #7487: the prohibition lapses when the call fails,
			// because the server only rejects a leader comment once the
			// no_action activity exists.
			"holds only while the call succeeds",
			"responsibility 3 applies",
			// one comment, not two — the fallback must not collide with the
			// one-comment-per-turn rule
			"never post a second comment",
		} {
			if !strings.Contains(compact, want) {
				t.Errorf("ownsParentStatus=%v: protocol missing %q\n--- protocol ---\n%s", ownsParentStatus, want, protocol)
			}
		}
	}
}

// Avoid "imported and not used: pgtype" if helpers above are the only users.
var _ pgtype.UUID
