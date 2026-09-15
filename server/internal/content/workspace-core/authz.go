// Package workspacecore answers one question for content modules: may this
// subject act inside this workspace?
//
// It deliberately does NOT answer whether a given object belongs to that
// workspace. The caller owns its own tables and has to check that itself. The
// distinction matters: a caller that assumes otherwise will read "allowed" and
// return an object that belongs to a different brand.
//
// The rules here were not invented for this package. They already ran inside
// the diagnostics handler, serving one module; this is that sequence extracted
// so the second content module does not copy it, and the third does not copy a
// drifted version of the copy.
//
// Contract, refusal mappings and invariants:
// specs/014-lt010-workspace-authorization/contracts/workspace-authorization.md
package workspacecore

import (
	"context"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// Reason says why a decision went the way it did. It is for the caller's
// records and for support, NOT for building a response: two different reasons
// deliberately map to one response so a refusal cannot be used to probe which
// workspaces exist. See the contract's mapping table.
type Reason string

const (
	ReasonNone        Reason = ""
	ReasonNoActor     Reason = "no_actor"
	ReasonNoWorkspace Reason = "no_workspace"
	ReasonNotMember   Reason = "not_member"
	ReasonRole        Reason = "role"
)

// Decision is the whole answer. Actor and Workspace are echoed back so a caller
// that acts on an allow does not have to re-derive what it was allowed for.
type Decision struct {
	Allowed   bool
	Reason    Reason
	Actor     string
	Workspace string
}

// Membership reads the caller's role in a workspace. Primitive types only, so
// this package depends on neither the generated db types nor the HTTP layer -
// which is what keeps the dependency pointing inward.
//
// found=false means no membership row. Whether that is because the workspace
// does not exist or because the subject is not in it is intentionally not
// distinguished: both must produce the same refusal.
type Membership interface {
	RoleFor(ctx context.Context, actor, workspace string) (role string, found bool, err error)
}

// Recorder is the diagnostics sink refusals are written to. An interface rather
// than the store itself so this package stays testable without a database.
type Recorder interface {
	Technical(ctx context.Context, event diagnostics.Event)
}

// Authorize decides whether actor may act in workspace with one of allowedRoles.
//
// Membership is read on EVERY call. Caching it would open a window in which a
// removed member still passes, and "the moment you are removed, you are out" is
// the property this exists to provide.
//
// Every refusal is recorded as a technical diagnostic event. That is a real
// integration, not a gesture at the onboarding contract: a refused request is
// exactly the kind of thing someone later asks "why was I locked out" about,
// and without this there would be nothing to answer with.
func Authorize(
	ctx context.Context,
	members Membership,
	recorder Recorder,
	actor, workspace string,
	allowedRoles ...string,
) Decision {
	decision := Decision{Actor: actor, Workspace: workspace}

	switch {
	case actor == "":
		// Not the same as being refused: nobody has claimed to be anybody yet.
		// The caller answers 401 here, never 404 - folding the two together
		// makes an expired session look like a missing resource.
		decision.Reason = ReasonNoActor
	case workspace == "":
		decision.Reason = ReasonNoWorkspace
	default:
		role, found, err := members.RoleFor(ctx, actor, workspace)
		switch {
		case err != nil || !found:
			// A lookup error lands here too. Treating "we could not tell" as a
			// refusal is the safe direction, and it keeps the response identical
			// to a genuine non-member, so an error cannot be used to probe.
			decision.Reason = ReasonNotMember
		case !roleAllowed(role, allowedRoles...):
			decision.Reason = ReasonRole
		default:
			decision.Allowed = true
			decision.Reason = ReasonNone
		}
	}

	if !decision.Allowed {
		record(ctx, recorder, decision)
	}
	return decision
}

func roleAllowed(role string, allowed ...string) bool {
	for _, candidate := range allowed {
		if role == candidate {
			return true
		}
	}
	return false
}

// record writes the refusal to the technical log. The event carries who, where
// and why, and nothing about the object that was being guarded - Sanitize
// enforces the field rules, but there is nothing to strip in the first place.
func record(ctx context.Context, recorder Recorder, decision Decision) {
	if recorder == nil {
		return
	}
	recorder.Technical(ctx, diagnostics.Event{
		ID:         diagnostics.NewID(),
		Workspace:  decision.Workspace,
		Actor:      decision.Actor,
		ActorKind:  "human",
		ObjectType: "account",
		Action:     "query",
		Outcome:    "failed",
		Code:       "AUTHORIZATION_DENIED",
		Component:  "workspace-core",
		Severity:   "warn",
		Step:       string(decision.Reason),
		Message:    "workspace authorization refused",
	})
}
