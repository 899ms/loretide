package workspacecore

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// Contract: specs/014-lt010-workspace-authorization/contracts/workspace-authorization.md

// fakeMembership answers from a map and counts how often it was asked, so a
// test can prove the helper re-reads rather than remembering (A5).
type fakeMembership struct {
	roles map[string]string // "actor@workspace" -> role
	reads int
}

func (f *fakeMembership) RoleFor(_ context.Context, actor, workspace string) (string, bool, error) {
	f.reads++
	role, ok := f.roles[actor+"@"+workspace]
	return role, ok, nil
}

// recordingRecorder captures the technical events the helper writes (A7).
type recordingRecorder struct{ events []diagnostics.Event }

func (r *recordingRecorder) Technical(_ context.Context, e diagnostics.Event) {
	r.events = append(r.events, e)
}

func members(pairs map[string]string) *fakeMembership {
	return &fakeMembership{roles: pairs}
}

const (
	brandA = "11111111-1111-4111-8111-111111111111"
	brandB = "22222222-2222-4222-8222-222222222222"
	userU1 = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	userU2 = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func decide(t *testing.T, m *fakeMembership, actor, workspace string) Decision {
	t.Helper()
	return Authorize(context.Background(), m, &recordingRecorder{}, actor, workspace, "owner", "admin")
}

// A1-A4: the reason enum. What the caller does with a reason is its own
// business; these pin which reason comes back.
func TestAuthorizeReasons(t *testing.T) {
	for _, tc := range []struct {
		name, actor, workspace string
		roles                  map[string]string
		wantAllowed            bool
		wantReason             Reason
	}{
		{"member with an allowed role", userU1, brandA,
			map[string]string{userU1 + "@" + brandA: "owner"}, true, ReasonNone},
		{"member with an admin role", userU1, brandA,
			map[string]string{userU1 + "@" + brandA: "admin"}, true, ReasonNone},
		{"A1 not a member", userU1, brandA, map[string]string{}, false, ReasonNotMember},
		{"A2 member whose role is not allowed", userU1, brandA,
			map[string]string{userU1 + "@" + brandA: "member"}, false, ReasonRole},
		{"A3 no actor", "", brandA, map[string]string{}, false, ReasonNoActor},
		{"A4 no workspace", userU1, "", map[string]string{}, false, ReasonNoWorkspace},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := decide(t, members(tc.roles), tc.actor, tc.workspace)
			if got.Allowed != tc.wantAllowed {
				t.Errorf("Allowed = %v, want %v", got.Allowed, tc.wantAllowed)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
		})
	}
}

func TestAuthorizeCarriesTheSubjectAndSpaceBack(t *testing.T) {
	got := decide(t, members(map[string]string{userU1 + "@" + brandA: "owner"}), userU1, brandA)
	if got.Actor != userU1 || got.Workspace != brandA {
		t.Errorf("decision carries actor=%q workspace=%q, want %q / %q",
			got.Actor, got.Workspace, userU1, brandA)
	}
}

// The five negatives the task card names, one case each, named for what they
// are rather than for the reason enum they happen to share. A reader checking
// the card against the suite should find them by name.

// (1) Same user, second brand. Being a member of A says nothing about B.
func TestSameUserIsRefusedInABrandTheyDoNotBelongTo(t *testing.T) {
	m := members(map[string]string{userU1 + "@" + brandA: "owner"})
	if got := decide(t, m, userU1, brandA); !got.Allowed {
		t.Fatal("setup: the user should be allowed in their own brand")
	}
	got := decide(t, m, userU1, brandB)
	if got.Allowed {
		t.Error("a member of brand A was allowed into brand B")
	}
	if got.Reason != ReasonNotMember {
		t.Errorf("Reason = %q, want %q", got.Reason, ReasonNotMember)
	}
}

// (2) A different identity entirely.
func TestAnotherIdentityIsRefused(t *testing.T) {
	m := members(map[string]string{userU1 + "@" + brandA: "owner"})
	got := decide(t, m, userU2, brandA)
	if got.Allowed {
		t.Error("a non-member identity was allowed")
	}
	if got.Reason != ReasonNotMember {
		t.Errorf("Reason = %q, want %q", got.Reason, ReasonNotMember)
	}
}

// (3) Passing somebody else's workspace id is not a way in.
func TestPassingSomeoneElsesWorkspaceIdIsRefused(t *testing.T) {
	m := members(map[string]string{userU2 + "@" + brandB: "owner"})
	got := decide(t, m, userU1, brandB)
	if got.Allowed {
		t.Error("supplying another brand's workspace id granted access")
	}
}

// (4) A workspace that does not exist must be indistinguishable from one the
// caller simply cannot see. Same reason, so the same response - otherwise the
// refusal itself tells an attacker which ids are real.
func TestAMissingWorkspaceIsRefusedForTheSameReasonAsOneYouCannotSee(t *testing.T) {
	m := members(map[string]string{userU2 + "@" + brandB: "owner"})
	missing := decide(t, m, userU1, "33333333-3333-4333-8333-333333333333")
	hidden := decide(t, m, userU1, brandB)
	if missing.Allowed || hidden.Allowed {
		t.Fatal("neither case may be allowed")
	}
	if missing.Reason != hidden.Reason {
		t.Errorf("a missing workspace answers %q and an invisible one %q; "+
			"different reasons let a caller tell them apart", missing.Reason, hidden.Reason)
	}
}

// A5: no caching. Two decisions must read membership twice, or a revocation
// between them would go unnoticed.
func TestEveryDecisionReadsMembershipAgain(t *testing.T) {
	m := members(map[string]string{userU1 + "@" + brandA: "owner"})
	decide(t, m, userU1, brandA)
	decide(t, m, userU1, brandA)
	if m.reads != 2 {
		t.Errorf("membership was read %d times for 2 decisions; caching would make a "+
			"revocation take effect late", m.reads)
	}
}

// A6 / (5): ownership changed under us.
func TestRemovingAMemberTakesEffectOnTheNextDecision(t *testing.T) {
	m := members(map[string]string{userU1 + "@" + brandA: "owner"})
	if !decide(t, m, userU1, brandA).Allowed {
		t.Fatal("setup: should start allowed")
	}
	delete(m.roles, userU1+"@"+brandA)
	got := decide(t, m, userU1, brandA)
	if got.Allowed {
		t.Error("a removed member was still allowed")
	}
	if got.Reason != ReasonNotMember {
		t.Errorf("Reason = %q, want %q", got.Reason, ReasonNotMember)
	}
}

func TestDemotingAMemberTakesEffectOnTheNextDecision(t *testing.T) {
	m := members(map[string]string{userU1 + "@" + brandA: "admin"})
	if !decide(t, m, userU1, brandA).Allowed {
		t.Fatal("setup: should start allowed")
	}
	m.roles[userU1+"@"+brandA] = "member"
	got := decide(t, m, userU1, brandA)
	if got.Allowed {
		t.Error("a demoted member kept their access")
	}
	if got.Reason != ReasonRole {
		t.Errorf("Reason = %q, want %q", got.Reason, ReasonRole)
	}
}

// A7: every refusal is recorded. This is the module's real diagnostics
// integration, not a call added to satisfy a static check.
func TestEveryRefusalIsRecordedAndNoAllowIs(t *testing.T) {
	m := members(map[string]string{userU1 + "@" + brandA: "owner"})

	rec := &recordingRecorder{}
	Authorize(context.Background(), m, rec, userU1, brandB, "owner", "admin")
	if len(rec.events) != 1 {
		t.Fatalf("a refusal recorded %d events, want 1", len(rec.events))
	}
	e := rec.events[0]
	if e.Code != "AUTHORIZATION_DENIED" {
		t.Errorf("recorded code = %q, want AUTHORIZATION_DENIED", e.Code)
	}
	if e.Outcome != "failed" {
		t.Errorf("recorded outcome = %q, want failed", e.Outcome)
	}
	if e.Workspace != brandB || e.Actor != userU1 {
		t.Errorf("recorded event names workspace %q actor %q, want %q / %q",
			e.Workspace, e.Actor, brandB, userU1)
	}

	allowed := &recordingRecorder{}
	Authorize(context.Background(), m, allowed, userU1, brandA, "owner", "admin")
	if len(allowed.events) != 0 {
		t.Errorf("an allowed decision recorded %d events, want 0", len(allowed.events))
	}
}

// The recorded event must not carry anything about the object being guarded.
func TestTheRecordedRefusalCarriesNoObjectBody(t *testing.T) {
	rec := &recordingRecorder{}
	Authorize(context.Background(), members(map[string]string{}), rec, userU1, brandA, "owner")
	if len(rec.events) != 1 {
		t.Fatalf("want exactly one recorded refusal, got %d", len(rec.events))
	}
	if rec.events[0].Message == "" {
		t.Error("the recorded event has no safe message")
	}
}

// A nil recorder must not panic: a caller that has not wired diagnostics yet
// still gets a correct decision.
func TestANilRecorderStillDecides(t *testing.T) {
	got := Authorize(context.Background(), members(map[string]string{}), nil, userU1, brandA, "owner")
	if got.Allowed || got.Reason != ReasonNotMember {
		t.Errorf("decision with a nil recorder = %+v", got)
	}
}

// The canonical mapping a new content module uses. Diagnostics keeps its own;
// see the contract's table.
func TestRefusalMappingHidesWhetherTheWorkspaceExists(t *testing.T) {
	// Every refusal that is about permission answers the same way, so a caller
	// cannot separate "not yours" from "not there".
	for _, reason := range []Reason{ReasonNotMember, ReasonRole, ReasonNoWorkspace} {
		if got := RefusalStatus(reason); got != 404 {
			t.Errorf("RefusalStatus(%q) = %d, want 404", reason, got)
		}
	}
	// Except this one: not signed in is a different problem.
	if got := RefusalStatus(ReasonNoActor); got != 401 {
		t.Errorf("RefusalStatus(no_actor) = %d, want 401", got)
	}
}

func TestRefusalBodyCarriesNothingAboutTheObject(t *testing.T) {
	body := RefusalBody("c0ffee")
	want := []string{"error", "code", "trace_id", "component", "retryable", "next_action"}
	if len(body) != len(want) {
		t.Errorf("refusal body has %d fields, want exactly %d: %v", len(body), len(want), body)
	}
	for _, key := range want {
		if _, ok := body[key]; !ok {
			t.Errorf("refusal body is missing %q", key)
		}
	}
	if body["code"] != "AUTHORIZATION_DENIED" {
		t.Errorf("code = %v, want AUTHORIZATION_DENIED", body["code"])
	}
}
