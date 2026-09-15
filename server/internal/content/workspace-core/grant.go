package workspacecore

import (
	"context"
	"slices"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// The second authorization layer: may THIS subject read THIS resource?
//
// The first layer (Authorize) answers whether a subject may act inside a brand
// at all. It stops at the brand boundary, and inside a brand there may be
// several accounts whose material is not meant to be shared between them. That
// gap is what this file closes.
//
// Like Authorize, it decides over facts the caller supplies and reads nothing
// itself: ownership arrives as a ResourceRef, the way membership arrives as a
// Membership. Nothing here queries a table, reads the clock or caches.
//
// Contract, decision order, matrix and the caching requirement:
// specs/020-lt016-material-grant-contract/contracts/material-grant.md

// PrincipalKind is what sort of subject is asking. It is part of the decision,
// not a label: a task's grant is not a person's, even when the ids collide.
type PrincipalKind string

const (
	PrincipalTask  PrincipalKind = "task"
	PrincipalHuman PrincipalKind = "human"
)

// Principal is who is asking. A task rather than an account, because one
// account runs many tasks and revocation has to be able to reach one of them
// without reaching the rest.
type Principal struct {
	Kind PrincipalKind
	ID   string
}

// ResourceKind is what sort of thing is being read. The first two are the ones
// the SOP already names; adding a third needs no migration, because grants are
// not stored yet.
type ResourceKind string

const (
	KindMaterial  ResourceKind = "material"
	KindKnowledge ResourceKind = "knowledge"
)

// ResourceKinds is the controlled set.
var ResourceKinds = []ResourceKind{KindMaterial, KindKnowledge}

// ResourceRef is where a resource sits: which brand, which account (empty means
// the brand shares it), what it is and which one.
//
// An empty Account is a real state and not a missing value: brand-wide material
// is ordinary, and without that level "account-private" would not mean anything
// in particular.
type ResourceRef struct {
	Workspace string
	Account   string
	Kind      ResourceKind
	ID        string
}

// Grant is one permission: who may read what, until when, and whether it has
// been taken back.
//
// Kinds being empty covers NOTHING. Reading an empty list as "everything" would
// turn a grant somebody forgot to fill in into the widest one in the system,
// which is the wrong direction to fail in. ExpiresAt being zero means no
// expiry; revocation and expiry are independent, and either alone is enough.
type Grant struct {
	Principal Principal
	Workspace string
	Account   string
	Kinds     []ResourceKind
	ExpiresAt time.Time
	Revoked   bool
}

// GrantReason says why a read was refused. Like Reason, it is for the record
// and for support, never for building a response: every refusal looks the same
// from outside, or a refusal would tell a caller which resources exist.
type GrantReason string

const (
	GrantReasonNone       GrantReason = ""
	GrantReasonIncomplete GrantReason = "incomplete_request"
	GrantReasonWorkspace  GrantReason = "workspace_mismatch"
	GrantReasonAccount    GrantReason = "account_mismatch"
	GrantReasonKind       GrantReason = "kind_mismatch"
	GrantReasonPrincipal  GrantReason = "principal_mismatch"
	GrantReasonRevoked    GrantReason = "revoked"
	GrantReasonExpired    GrantReason = "expired"
	GrantReasonNoGrant    GrantReason = "no_grant"
)

// GrantDecision is the whole answer, shaped like Decision so a caller that has
// learned one has learned both.
type GrantDecision struct {
	Allowed   bool
	Reason    GrantReason
	Principal Principal
	Workspace string
}

// ReadRequest is everything the decision needs.
//
// Workspace is the brand the caller has ALREADY been authorized in. This layer
// does not redo that check; it assumes it happened, and refuses anything whose
// resource belongs elsewhere.
//
// Now is passed in rather than read, so expiry is testable without waiting and
// a caller cannot get a different answer from a clock that drifted.
type ReadRequest struct {
	Principal Principal
	Workspace string
	Resource  ResourceRef
	Now       time.Time
	Grants    []Grant
}

// CanRead decides whether the principal may read the resource.
//
// Nothing here looks at the account's persona prompt or any other expression
// setting, and nothing may: a prompt is configuration, not a credential. Two
// accounts whose prompts are identical are still two accounts. The case that
// holds this lives in the handler package, where a real account and a real
// prompt are both reachable.
func CanRead(ctx context.Context, recorder Recorder, request ReadRequest) GrantDecision {
	decision := GrantDecision{Principal: request.Principal, Workspace: request.Workspace}

	switch {
	case request.Principal.ID == "" || request.Principal.Kind == "" ||
		request.Workspace == "" || request.Resource.Workspace == "" ||
		request.Resource.Kind == "" || request.Resource.ID == "":
		// Not a refusal of a real request: the request did not say enough to be
		// decided, and filling a blank in with "any" is how a guard is lost.
		decision.Reason = GrantReasonIncomplete
	case request.Resource.Workspace != request.Workspace:
		decision.Reason = GrantReasonWorkspace
	case request.Resource.Account == "":
		// Brand-level material. Membership in the brand is the whole condition,
		// and Authorize already established it.
		decision.Allowed = true
		decision.Reason = GrantReasonNone
	default:
		decision.Reason = matchGrant(request)
		decision.Allowed = decision.Reason == GrantReasonNone
	}

	if !decision.Allowed {
		recordGrantRefusal(ctx, recorder, decision)
	}
	return decision
}

// matchGrant returns GrantReasonNone when some grant covers the request, and
// otherwise the NEAREST miss.
//
// Nearest rather than first: the reason never reaches a response, so its only
// job is to be useful later, and "you had a grant that expired an hour ago" is
// a different conversation from "you never had one". The ordering below is that
// judgement written down.
func matchGrant(request ReadRequest) GrantReason {
	if len(request.Grants) == 0 {
		return GrantReasonNoGrant
	}
	nearest := GrantReasonNoGrant
	for _, grant := range request.Grants {
		switch {
		case grant.Principal != request.Principal:
			nearest = closer(nearest, GrantReasonPrincipal)
		case grant.Workspace != request.Workspace:
			nearest = closer(nearest, GrantReasonWorkspace)
		case grant.Account != "" && grant.Account != request.Resource.Account:
			// An empty grant account is brand-wide and covers every account in
			// it; a named one has to match.
			nearest = closer(nearest, GrantReasonAccount)
		case !slices.Contains(grant.Kinds, request.Resource.Kind):
			nearest = closer(nearest, GrantReasonKind)
		case grant.Revoked:
			// Checked before expiry on purpose: a revoked grant must not keep
			// working until its own clock runs out.
			nearest = closer(nearest, GrantReasonRevoked)
		case !grant.ExpiresAt.IsZero() && !grant.ExpiresAt.After(request.Now):
			nearest = closer(nearest, GrantReasonExpired)
		default:
			return GrantReasonNone
		}
	}
	return nearest
}

// grantReasonRank orders misses from vaguest to nearest.
var grantReasonRank = []GrantReason{
	GrantReasonNoGrant, GrantReasonPrincipal, GrantReasonWorkspace,
	GrantReasonAccount, GrantReasonKind, GrantReasonExpired, GrantReasonRevoked,
}

func closer(current, candidate GrantReason) GrantReason {
	if slices.Index(grantReasonRank, candidate) > slices.Index(grantReasonRank, current) {
		return candidate
	}
	return current
}

// recordGrantRefusal writes the refusal to the technical log.
//
// It names who was refused and why, and NOTHING about the resource - not its
// id, not which account owns it. A caller who cannot read a resource must not
// learn from the log that it exists, and the account id would say exactly that.
func recordGrantRefusal(ctx context.Context, recorder Recorder, decision GrantDecision) {
	if recorder == nil {
		return
	}
	recorder.Technical(ctx, diagnostics.Event{
		ID:         diagnostics.NewID(),
		Workspace:  decision.Workspace,
		Actor:      decision.Principal.ID,
		ActorKind:  string(decision.Principal.Kind),
		ObjectType: "resource",
		Action:     "query",
		Outcome:    "failed",
		Code:       "AUTHORIZATION_DENIED",
		Component:  "workspace-core",
		Severity:   "warn",
		Step:       string(decision.Reason),
		Message:    "resource read refused",
	})
}
