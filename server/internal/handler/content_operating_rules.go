package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
)

// The brand's operating rules (specs/029, SOP 3.2): weekly cadence, channel
// templates, who reviews, and when to go and look at the numbers.
//
// Two endpoints and no path parameter: which brand is decided by the
// X-Workspace-ID header, the same way every other brand-scoped read is. That
// means workflow step 12's "parameter value != context value" test has no
// subject here - there is no parameter to disagree with the context. The
// account homepage endpoint next door DOES have one, and does have that test.
//
// PUT rather than PATCH-with-a-settings-field, because the workspace update
// query assigns the settings column wholesale: a partial write there would
// delete the brand's timezone and precheck switch. LT-014 reached the same
// conclusion for the account scope and said so in router.go; this follows it.

func (h *Handler) operatingRulesScope(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	workspace := h.resolveWorkspaceID(r)
	actor := r.Header.Get("X-User-ID")
	decision := workspacecore.Authorize(r.Context(), h.diagnosticMembership(),
		h.diagnosticRefusalRecorder(), actor, workspace, "owner", "admin", "member")
	if !decision.Allowed {
		writeJSON(w, workspacecore.RefusalStatus(decision.Reason),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
		return "", "", false
	}
	return workspace, actor, true
}

type operatingRulesDatabase struct {
	dbExecutor
	txStarter
}

func (d operatingRulesDatabase) Begin(ctx context.Context) (pgx.Tx, error) {
	if d.txStarter == nil {
		return nil, workspacecore.ErrStorage
	}
	return d.txStarter.Begin(ctx)
}

func (d operatingRulesDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if d.dbExecutor == nil {
		return errorRow{err: workspacecore.ErrStorage}
	}
	return d.dbExecutor.QueryRow(ctx, sql, args...)
}

func (h *Handler) operatingRulesStore() *workspacecore.Store {
	var diagnosticStore workspacecore.DiagnosticStore
	build := ""
	if h.ContentDiagnostics != nil {
		diagnosticStore = h.ContentDiagnostics.Store
		build = h.ContentDiagnostics.Build
	}
	return &workspacecore.Store{
		DB:          operatingRulesDatabase{dbExecutor: h.DB, txStarter: h.TxStarter},
		Diagnostics: diagnosticStore,
		// The same fence the diagnostics store holds, handed over directly so
		// the write transaction takes the workspace delete lock itself rather
		// than inheriting it from whether it happened to audit first (#104).
		Guard: newContentDiagnosticsWorkspaceWriteGuard(h.Queries),
		Build: build,
	}
}

// GetContentOperatingRules returns the brand's rules with defaults filled.
//
// Filling happens on the way out and nothing is written back: reading a
// workspace must not modify it. That is the same rule timezoneFilled and
// autoPrecheckFilled follow a few hundred lines away in workspace.go.
func (h *Handler) GetContentOperatingRules(w http.ResponseWriter, r *http.Request) {
	workspace, _, ok := h.operatingRulesScope(w, r)
	if !ok {
		return
	}
	rules, err := h.operatingRulesStore().ReadRules(r.Context(), workspace)
	if err != nil {
		h.operatingRulesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

// SetContentOperatingRules validates and stores the rules.
//
// The whole set arrives at once because that is how it is edited - one form,
// one sitting - but only this card's key is written. Every other key in the
// settings blob is left exactly as it was.
func (h *Handler) SetContentOperatingRules(w http.ResponseWriter, r *http.Request) {
	workspace, actor, ok := h.operatingRulesScope(w, r)
	if !ok {
		return
	}
	var body workspacecore.Rules
	if !decodeOperatingRulesBody(w, r, &body) {
		h.operatingRulesError(w, workspacecore.ErrInvalid)
		return
	}
	written, err := h.operatingRulesStore().WriteRules(r.Context(), workspace, actor, body)
	if err != nil {
		h.operatingRulesError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, written)
}

func decodeOperatingRulesBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4*1024*1024))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil
}

// operatingRulesError maps the module's errors. A refusal and a missing brand
// answer identically, so a caller cannot use the response to learn that a
// workspace exists.
func (h *Handler) operatingRulesError(w http.ResponseWriter, err error) {
	var fieldErr workspacecore.FieldError
	switch {
	case errors.As(err, &fieldErr):
		h.operatingRulesDiagnosticError(w, http.StatusBadRequest,
			map[string]any{"field": fieldErr.Field, "reason": fieldErr.Reason})
	case errors.Is(err, workspacecore.ErrInvalid):
		h.operatingRulesDiagnosticError(w, http.StatusBadRequest, nil)
	case errors.Is(err, workspacecore.ErrNotFound), errors.Is(err, diagnostics.ErrDenied):
		writeJSON(w, workspacecore.RefusalStatus(workspacecore.ReasonNotMember),
			workspacecore.RefusalBody(w.Header().Get("X-Diagnostic-Trace")))
	default:
		writeError(w, http.StatusServiceUnavailable, "operating rules storage unavailable")
	}
}

func (h *Handler) operatingRulesDiagnosticError(w http.ResponseWriter, status int, detail map[string]any) {
	event := diagnostics.Sanitize(diagnostics.Event{
		Code: "INPUT_CONFLICT", Component: "workspace-core",
		Trace: w.Header().Get("X-Diagnostic-Trace"),
	})
	body := map[string]any{
		"error": event.Message, "code": event.Code, "trace_id": event.Trace,
		"component": event.Component, "retryable": false,
		"next_action": event.Next,
	}
	for key, value := range detail {
		body[key] = value
	}
	writeJSON(w, status, body)
}
