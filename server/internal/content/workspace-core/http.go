package workspacecore

import (
	"net/http"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// RefusalStatus is the status a NEW content module answers with when Authorize
// refuses. It is 404 for every refusal reason except a missing subject.
//
// Not 403: answering 403 concedes that the thing exists and you merely cannot
// have it, which tells an unauthorized caller which workspace ids are real.
// "Not a member" and "no such workspace" therefore produce one answer.
//
// Diagnostics is the exception and keeps its own mapping (404 for a non-member,
// 403 for role and account) because those responses already shipped and were
// verified; changing them would be a regression dressed up as consistency. The
// contract's mapping table records which caller uses which.
func RefusalStatus(reason Reason) int {
	if reason == ReasonNoActor {
		// Unauthenticated is a different problem with a different fix: sign in.
		return http.StatusUnauthorized
	}
	return http.StatusNotFound
}

// RefusalBody is what a refused caller receives. Every field comes from a
// sanitized diagnostic event, so the shape matches what the rest of the product
// already returns for a denial and carries nothing about the guarded object -
// no name, no settings, not even the id echoed back.
func RefusalBody(traceID string) map[string]any {
	event := diagnostics.Sanitize(diagnostics.Event{
		Code:      "AUTHORIZATION_DENIED",
		Component: "workspace-core",
		Trace:     traceID,
	})
	return map[string]any{
		"error":       event.Message,
		"code":        event.Code,
		"trace_id":    event.Trace,
		"component":   event.Component,
		"retryable":   event.Retryable,
		"next_action": event.Next,
	}
}
