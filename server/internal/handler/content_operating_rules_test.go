package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Contract: specs/029-operating-rules/contracts/operating-rules.md
//
// What these are for: the module's own tests prove the rules, and these prove
// the rules survive a real workspace row - the settings column, the other keys
// already on it, and the delete fence.

// rulesWorkspace makes a brand whose settings already carry a timezone and a
// precheck switch, because the thing most likely to go wrong here is a write
// that takes them with it.
func rulesWorkspace(t *testing.T, slug string) string {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Brand " + slug, "slug": slug, "description": "operating rules test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`,
		wsID, testUserID)
	dbfx.Exec(t, `UPDATE workspace SET settings =
		'{"loretide.timezone":"Asia/Shanghai","loretide.auto_precheck":false}'::jsonb
		WHERE id = $1`, wsID)
	t.Cleanup(func() {
		background := context.Background()
		_, _ = testPool.Exec(background, `DELETE FROM content_operation_audit WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(background, `DELETE FROM content_account WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(background, `DELETE FROM workspace WHERE id = $1`, wsID)
	})
	return wsID
}

func rulesHandler(t *testing.T) *Handler {
	t.Helper()
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)
	return &h
}

func rulesCall(t *testing.T, handler http.HandlerFunc, method, wsID, path, body string) *testutil.Response {
	t.Helper()
	req := testutil.WithHeaders(testutil.JSONRequest(method, path, body),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	return testutil.Call(t, handler, req)
}

func settingsKey(t *testing.T, wsID, key string) (string, bool) {
	t.Helper()
	var value *string
	if err := testPool.QueryRow(t.Context(),
		`SELECT settings->>$2 FROM workspace WHERE id = $1`, wsID, key).Scan(&value); err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	if value == nil {
		return "", false
	}
	return *value, true
}

// A brand that has never set anything still renders a page: every field comes
// back with a value, and none of them is invented.
func TestOperatingRulesReadFillsDefaultsWithoutWritingBack(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := rulesHandler(t)
	wsID := rulesWorkspace(t, "rules-defaults")

	var before, after time.Time
	if err := testPool.QueryRow(t.Context(),
		`SELECT updated_at FROM workspace WHERE id = $1`, wsID).Scan(&before); err != nil {
		t.Fatal(err)
	}

	var rules workspacecore.Rules
	rulesCall(t, h.GetContentOperatingRules, "GET", wsID, "/api/operating-rules", "").
		Want(http.StatusOK).JSON(&rules)

	if rules.ReviewRule != workspacecore.ReviewRuleSelf {
		t.Errorf("review rule = %q, want self", rules.ReviewRule)
	}
	// Nothing invented: no cadence and no observation window.
	if len(rules.Cadence) != 0 {
		t.Errorf("a brand that set nothing has a cadence: %v", rules.Cadence)
	}
	if rules.Observation.Default != nil {
		t.Errorf("a brand that set nothing has an observation window: %v", *rules.Observation.Default)
	}

	// Reading a workspace must not modify it: the defaults are filled on the
	// way out, exactly like timezoneFilled and autoPrecheckFilled.
	if err := testPool.QueryRow(t.Context(),
		`SELECT updated_at FROM workspace WHERE id = $1`, wsID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(before) {
		t.Errorf("a read touched the row: %v -> %v", before, after)
	}
	if _, stored := settingsKey(t, wsID, workspacecore.OperatingRulesKey); stored {
		t.Error("a read wrote the rules key back into settings")
	}
}

// The reason this card has its own endpoint instead of a field on the
// workspace PATCH: that query assigns the settings column wholesale.
func TestWritingRulesLeavesEveryOtherSettingAlone(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := rulesHandler(t)
	wsID := rulesWorkspace(t, "rules-merge")

	rulesCall(t, h.SetContentOperatingRules, "PUT", wsID, "/api/operating-rules",
		`{"cadence":{"xiaohongshu":3},"observation":{"default":14}}`).Want(http.StatusOK)

	timezone, hasTimezone := settingsKey(t, wsID, "loretide.timezone")
	precheck, hasPrecheck := settingsKey(t, wsID, "loretide.auto_precheck")
	if !hasTimezone || timezone != "Asia/Shanghai" {
		t.Errorf("the brand's timezone became %q (present=%v)", timezone, hasTimezone)
	}
	// Stored as false on purpose in the fixture: a merge bug that dropped the
	// key and a default that flipped it back on would look the same if this
	// were true.
	if !hasPrecheck || precheck != "false" {
		t.Errorf("the brand's precheck switch became %q (present=%v)", precheck, hasPrecheck)
	}
}

// The single silent-corruption risk on this card, at the boundary this time.
func TestAStoredZeroSurvivesTheBoundaryAndAnAbsentValueStaysAbsent(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := rulesHandler(t)
	wsID := rulesWorkspace(t, "rules-zero")

	rulesCall(t, h.SetContentOperatingRules, "PUT", wsID, "/api/operating-rules",
		`{"cadence":{"wechat_mp":0},"observation":{"default":0}}`).Want(http.StatusOK)

	var rules workspacecore.Rules
	rulesCall(t, h.GetContentOperatingRules, "GET", wsID, "/api/operating-rules", "").
		Want(http.StatusOK).JSON(&rules)

	value, stored := workspacecore.ReadCadence(rules, "wechat_mp")
	if value != 0 || !stored {
		t.Errorf("a stored 0 came back as (%d, %v), want (0, true)", value, stored)
	}
	if _, absent := workspacecore.ReadCadence(rules, "douyin"); absent {
		t.Error("a channel nobody set came back as stored")
	}
	days, source := workspacecore.ReadObservation(rules, "douyin")
	if days != 0 || source != workspacecore.ObservationFromGlobal {
		t.Errorf("a stored 0-day window came back as (%d, %q), want (0, global)", days, source)
	}
}

func TestOperatingRulesRefusalsNameTheField(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := rulesHandler(t)
	wsID := rulesWorkspace(t, "rules-refusals")

	for _, tc := range []struct {
		name  string
		body  string
		field string
	}{
		{"negative cadence", `{"cadence":{"xiaohongshu":-1},"observation":{}}`, "cadence.xiaohongshu"},
		{"unknown channel", `{"cadence":{"twitter":1},"observation":{}}`, "cadence"},
		{"team review", `{"review_rule":"team","observation":{}}`, "review_rule"},
		{"negative window", `{"observation":{"default":-1}}`, "observation.default"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			rulesCall(t, h.SetContentOperatingRules, "PUT", wsID, "/api/operating-rules", tc.body).
				Want(http.StatusBadRequest).JSON(&body)
			if body["field"] != tc.field {
				t.Errorf("named %v, want %q", body["field"], tc.field)
			}
		})
	}
}

// A non-member and a workspace that is not there answer byte for byte the
// same, so a refusal cannot be used to learn which brands exist.
func TestOperatingRulesHideTheBrandFromOutsiders(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := rulesHandler(t)
	foreign := rulesWorkspace(t, "rules-foreign")
	// Not a member of it.
	dbfx.Exec(t, `DELETE FROM member WHERE workspace_id = $1 AND user_id = $2`, foreign, testUserID)

	var refused, missing map[string]any
	rulesCall(t, h.GetContentOperatingRules, "GET", foreign, "/api/operating-rules", "").
		Want(http.StatusNotFound).JSON(&refused)
	rulesCall(t, h.GetContentOperatingRules, "GET", "ws-does-not-exist", "/api/operating-rules", "").
		Want(http.StatusNotFound).JSON(&missing)

	delete(refused, "trace_id")
	delete(missing, "trace_id")
	left, _ := json.Marshal(refused)
	right, _ := json.Marshal(missing)
	if string(left) != string(right) {
		t.Errorf("the two refusals differ:\nforeign: %s\nmissing: %s", left, right)
	}
}

// The delete/write protocol the other content modules follow (#104). The
// settings live on the workspace row itself, so the race here is narrower than
// for a content table - but a write that lands between the delete committing
// and the request finishing would still resurrect a row, and the fence is what
// stops it.
func TestOperatingRulesWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := rulesHandler(t)
	store := h.operatingRulesStore()
	wsID := rulesWorkspace(t, "rules-fence")

	accountID := fmt.Sprintf("acct-fence-%d", time.Now().UnixNano())
	dbfx.Exec(t, `INSERT INTO content_account
		(account_id, workspace_id, platform, display_name)
		VALUES ($1,$2,'xiaohongshu','Fence')`, accountID, wsID)

	rules := workspacecore.DefaultRules()
	rules.Cadence["xiaohongshu"] = 3
	if _, err := store.WriteRules(ctx, wsID, testUserID, rules); err != nil {
		t.Fatalf("write while the workspace exists: %v", err)
	}
	if err := store.WriteHomepage(ctx, wsID, testUserID, accountID, "https://example.invalid/me"); err != nil {
		t.Fatalf("homepage while the workspace exists: %v", err)
	}

	// The delete commits on its own connection, exactly as a workspace
	// deletion that finished just before the next request arrived.
	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id = $1`, wsID); err != nil {
		t.Fatal(err)
	}

	for _, write := range []struct {
		name string
		call func() error
	}{
		{"rules", func() error {
			_, err := store.WriteRules(ctx, wsID, testUserID, rules)
			return err
		}},
		{"homepage", func() error {
			return store.WriteHomepage(ctx, wsID, testUserID, accountID, "https://example.invalid/after")
		}},
	} {
		t.Run(write.name, func(t *testing.T) {
			err := write.call()
			if !errors.Is(err, workspacecore.ErrNotFound) {
				t.Fatalf("after the delete = %v, want ErrNotFound", err)
			}
		})
	}

	// And nothing was left half-written on the account row either.
	var remaining int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM content_account WHERE account_id = $1
		 AND settings->>'loretide.homepage' = 'https://example.invalid/after'`,
		accountID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("a homepage landed after the workspace was deleted")
	}
	_, _ = testPool.Exec(context.Background(), `DELETE FROM content_account WHERE account_id = $1`, accountID)
}

// FR-015 / SC-006: only the channel note may enter a model's context.
func TestOnlyTheChannelNoteLeavesForAModel(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := rulesHandler(t)
	wsID := rulesWorkspace(t, "rules-context")

	rulesCall(t, h.SetContentOperatingRules, "PUT", wsID, "/api/operating-rules",
		`{"cadence":{"xiaohongshu":3},"templates":{"xiaohongshu":{"note":"标题 20 字内"}},
		  "observation":{"default":14}}`).Want(http.StatusOK)

	stored, err := h.operatingRulesStore().ReadRules(t.Context(), wsID)
	if err != nil {
		t.Fatal(err)
	}
	note := workspacecore.TemplateNoteFor(stored, "xiaohongshu")
	if note != "标题 20 字内" {
		t.Fatalf("note = %q", note)
	}
	for _, leaked := range []string{"3", "14"} {
		if note == leaked {
			t.Errorf("the model context carries %q", leaked)
		}
	}
}
