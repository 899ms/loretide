package workspacecore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// Contract: specs/020-lt016-material-grant-contract/contracts/material-grant.md
//
// The card's whole output is a decision table, so the table is the test. Each
// row states its expectation next to its inputs; nothing is asserted somewhere
// else where you would have to hold two files in your head to check it.

var (
	taskOne   = Principal{Kind: PrincipalTask, ID: "task-1"}
	taskTwo   = Principal{Kind: PrincipalTask, ID: "task-2"}
	humanOne  = Principal{Kind: PrincipalHuman, ID: "task-1"} // same id, different kind
	brandW    = "workspace-w"
	brandX    = "workspace-x"
	accountA  = "account-a"
	accountB  = "account-b"
	now       = time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	yesterday = now.Add(-24 * time.Hour)
	tomorrow  = now.Add(24 * time.Hour)
)

func materialIn(workspace, account string) ResourceRef {
	return ResourceRef{Workspace: workspace, Account: account, Kind: KindMaterial, ID: "res-1"}
}

func grantFor(p Principal, workspace, account string, kinds ...ResourceKind) Grant {
	return Grant{Principal: p, Workspace: workspace, Account: account, Kinds: kinds, ExpiresAt: tomorrow}
}

func TestReadPermissionMatrix(t *testing.T) {
	for _, tc := range []struct {
		name      string
		principal Principal
		resource  ResourceRef
		grants    []Grant
		allowed   bool
		reason    GrantReason
	}{
		{
			name:      "1 the grant names this account and this kind",
			principal: taskOne, resource: materialIn(brandW, accountA),
			grants:  []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
			allowed: true, reason: GrantReasonNone,
		},
		{
			name:      "2 another account in the same brand",
			principal: taskOne, resource: materialIn(brandW, accountB),
			grants:  []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
			allowed: false, reason: GrantReasonAccount,
		},
		{
			name:      "3 another brand entirely",
			principal: taskOne, resource: materialIn(brandX, accountA),
			grants:  []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
			allowed: false, reason: GrantReasonWorkspace,
		},
		{
			name:      "4 a kind the grant does not cover",
			principal: taskOne,
			resource:  ResourceRef{Workspace: brandW, Account: accountA, Kind: KindKnowledge, ID: "res-1"},
			grants:    []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
			allowed:   false, reason: GrantReasonKind,
		},
		{
			name:      "5 someone else's grant",
			principal: taskTwo, resource: materialIn(brandW, accountA),
			grants:  []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
			allowed: false, reason: GrantReasonPrincipal,
		},
		{
			name:      "6 revoked",
			principal: taskOne, resource: materialIn(brandW, accountA),
			grants: []Grant{func() Grant {
				g := grantFor(taskOne, brandW, accountA, KindMaterial)
				g.Revoked = true
				return g
			}()},
			allowed: false, reason: GrantReasonRevoked,
		},
		{
			name:      "7 expired",
			principal: taskOne, resource: materialIn(brandW, accountA),
			grants: []Grant{func() Grant {
				g := grantFor(taskOne, brandW, accountA, KindMaterial)
				g.ExpiresAt = yesterday
				return g
			}()},
			allowed: false, reason: GrantReasonExpired,
		},
		{
			name:      "8 brand-level resource needs no grant",
			principal: taskOne, resource: materialIn(brandW, ""),
			grants:  nil,
			allowed: true, reason: GrantReasonNone,
		},
		{
			name:      "9 a brand-wide grant covers an account's resource",
			principal: taskOne, resource: materialIn(brandW, accountA),
			grants:  []Grant{grantFor(taskOne, brandW, "", KindMaterial)},
			allowed: true, reason: GrantReasonNone,
		},
		{
			// Empty is not "everything". A grant written without kinds is a
			// mistake, and reading it as the widest possible grant turns that
			// mistake into the most dangerous value in the system.
			name:      "10 an empty kind list covers nothing",
			principal: taskOne, resource: materialIn(brandW, accountA),
			grants:  []Grant{grantFor(taskOne, brandW, accountA)},
			allowed: false, reason: GrantReasonKind,
		},
		{
			name:      "11 one covering grant among several",
			principal: taskOne, resource: materialIn(brandW, accountA),
			grants: []Grant{
				grantFor(taskOne, brandW, accountB, KindMaterial),
				grantFor(taskOne, brandW, accountA, KindMaterial),
			},
			allowed: true, reason: GrantReasonNone,
		},
		{
			// Same id, different kind of subject. A task's grant is not a
			// person's, even when an id happens to collide.
			name:      "12 a human holding a task's grant",
			principal: humanOne, resource: materialIn(brandW, accountA),
			grants:  []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
			allowed: false, reason: GrantReasonPrincipal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := CanRead(context.Background(), nil, ReadRequest{
				Principal: tc.principal, Workspace: brandW,
				Resource: tc.resource, Now: now, Grants: tc.grants,
			})
			if decision.Allowed != tc.allowed {
				t.Errorf("allowed = %v, want %v (reason %q)", decision.Allowed, tc.allowed, decision.Reason)
			}
			if decision.Reason != tc.reason {
				t.Errorf("reason = %q, want %q", decision.Reason, tc.reason)
			}
		})
	}
}

// Incomplete input is refused rather than guessed at. A ref missing its
// workspace is not "any workspace".
func TestAnIncompleteRequestIsRefused(t *testing.T) {
	complete := ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now,
		Grants: []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
	}
	if !CanRead(context.Background(), nil, complete).Allowed {
		t.Fatal("the control case must be allowed, or the cases below prove nothing")
	}

	for name, damage := range map[string]func(*ReadRequest){
		"no principal id":       func(r *ReadRequest) { r.Principal.ID = "" },
		"no principal kind":     func(r *ReadRequest) { r.Principal.Kind = "" },
		"no authorized brand":   func(r *ReadRequest) { r.Workspace = "" },
		"resource has no brand": func(r *ReadRequest) { r.Resource.Workspace = "" },
		"resource has no kind":  func(r *ReadRequest) { r.Resource.Kind = "" },
		"resource has no id":    func(r *ReadRequest) { r.Resource.ID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			request := complete
			request.Grants = append([]Grant(nil), complete.Grants...)
			damage(&request)
			decision := CanRead(context.Background(), nil, request)
			if decision.Allowed {
				t.Errorf("an incomplete request was allowed")
			}
			if decision.Reason != GrantReasonIncomplete {
				t.Errorf("reason = %q, want %q", decision.Reason, GrantReasonIncomplete)
			}
		})
	}
}

// A zero Grant must not be a key to anything. It is what a caller gets from a
// misread row or an unset field.
func TestAZeroGrantOpensNothing(t *testing.T) {
	decision := CanRead(context.Background(), nil, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now,
		Grants: []Grant{{}},
	})
	if decision.Allowed {
		t.Error("a zero-valued grant allowed a read")
	}
}

// Zero ExpiresAt means no expiry, which is different from "expired at the zero
// time" — the latter would make every open-ended grant dead on arrival.
func TestAGrantWithNoExpiryDoesNotExpire(t *testing.T) {
	grant := grantFor(taskOne, brandW, accountA, KindMaterial)
	grant.ExpiresAt = time.Time{}

	decision := CanRead(context.Background(), nil, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now, Grants: []Grant{grant},
	})
	if !decision.Allowed {
		t.Errorf("an open-ended grant was refused: %q", decision.Reason)
	}
}

// Revocation beats a valid expiry. Reading them in the other order would let a
// revoked grant keep working until its own clock ran out.
func TestRevocationBeatsAValidExpiry(t *testing.T) {
	grant := grantFor(taskOne, brandW, accountA, KindMaterial)
	grant.ExpiresAt = tomorrow
	grant.Revoked = true

	decision := CanRead(context.Background(), nil, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now, Grants: []Grant{grant},
	})
	if decision.Allowed || decision.Reason != GrantReasonRevoked {
		t.Errorf("allowed=%v reason=%q", decision.Allowed, decision.Reason)
	}
}

// The reason names the nearest miss, not the vaguest one. It never reaches a
// response — every refusal looks the same from outside — so a test is the only
// thing that can notice it is wrong, and "you had a grant that expired an hour
// ago" is a different support conversation from "you never had one".
func TestTheReasonNamesTheNearestMiss(t *testing.T) {
	expired := grantFor(taskOne, brandW, accountA, KindMaterial)
	expired.ExpiresAt = yesterday

	decision := CanRead(context.Background(), nil, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now,
		Grants: []Grant{
			grantFor(taskOne, brandW, accountB, KindMaterial), // wrong account
			expired, // right in every way but time
		},
	})
	if decision.Reason != GrantReasonExpired {
		t.Errorf("reason = %q, want %q — the nearest miss is the useful one",
			decision.Reason, GrantReasonExpired)
	}
}

// With no grants at all the reason is that, and not a mismatch invented from
// nothing.
func TestNoGrantsAtAllSaysSo(t *testing.T) {
	decision := CanRead(context.Background(), nil, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now,
	})
	if decision.Reason != GrantReasonNoGrant {
		t.Errorf("reason = %q, want %q", decision.Reason, GrantReasonNoGrant)
	}
}

// The resource being guarded must not appear in what gets recorded. The event
// says who was refused and why; anything else would put the id of a resource
// the caller could not read into a log they may be able to.
func TestARefusalRecordsNothingAboutTheResource(t *testing.T) {
	recorder := &recordingRecorder{}

	CanRead(context.Background(), recorder, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: ResourceRef{Workspace: brandW, Account: "secret-account", Kind: KindMaterial, ID: "secret-resource"},
		Now:      now,
	})

	if len(recorder.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(recorder.events))
	}
	rendered := recorder.events[0].Workspace + "|" + recorder.events[0].Account + "|" +
		recorder.events[0].Actor + "|" + recorder.events[0].ObjectID + "|" +
		recorder.events[0].Message + "|" + recorder.events[0].Step
	for _, secret := range []string{"secret-resource", "secret-account"} {
		if strings.Contains(rendered, secret) {
			t.Errorf("the refusal event carried %q: %s", secret, rendered)
		}
	}
}

// An allowed read records nothing. Refusals are the interesting half; logging
// every successful read would bury them.
func TestAnAllowedReadRecordsNothing(t *testing.T) {
	recorder := &recordingRecorder{}

	CanRead(context.Background(), recorder, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now,
		Grants: []Grant{grantFor(taskOne, brandW, accountA, KindMaterial)},
	})

	if len(recorder.events) != 0 {
		t.Errorf("an allowed read recorded %d events", len(recorder.events))
	}
}

// Issue #108, the grant half. Both timestamps were zero, so a refused read
// rendered in the panel as having happened at the epoch, and the component was
// not on the sanitizer's allowlist, so it rendered as "unknown" besides.
//
// `Now` in the request is the clock the DECISION uses (expiry); the event's own
// timestamps are when it was recorded, which is why this asserts against the
// wall clock rather than against `now`.
func TestARefusalRecordsWhenItHappenedAndFromWhere(t *testing.T) {
	before := time.Now().UTC().Add(-time.Second)
	recorder := &recordingRecorder{}

	CanRead(context.Background(), recorder, ReadRequest{
		Principal: taskOne, Workspace: brandW,
		Resource: materialIn(brandW, accountA), Now: now,
	})

	if len(recorder.events) != 1 {
		t.Fatalf("recorded %d events, want 1", len(recorder.events))
	}
	after := time.Now().UTC().Add(time.Second)
	event := recorder.events[0]

	for _, stamp := range []struct {
		name string
		at   time.Time
	}{
		{"occurred_at", event.Occurred},
		{"received_at", event.Received},
	} {
		if stamp.at.IsZero() {
			t.Errorf("%s is the zero time; the refusal reads as if it never happened", stamp.name)
			continue
		}
		if stamp.at.Before(before) || stamp.at.After(after) {
			t.Errorf("%s = %s, outside [%s, %s]", stamp.name, stamp.at, before, after)
		}
	}

	if got := diagnostics.Sanitize(event).Component; got != "workspace-core" {
		t.Errorf("sanitized component = %q, want workspace-core", got)
	}
}
