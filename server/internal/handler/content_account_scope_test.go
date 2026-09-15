package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Contract: specs/018-lt014-account-scope-preference/contracts/account-scope.md

const scopeKey = "loretide.scope"

func setScope(t *testing.T, wsID, accountID, scope string) *testutil.Response {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"scope": scope})
	if err != nil {
		t.Fatal(err)
	}
	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest("PUT", "/api/content-accounts/"+accountID+"/scope", string(payload)),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID), "id", accountID)
	return testutil.Call(t, testHandler.SetAccountScope, req)
}

// storedSettings reads the row itself, not the response. Every "did this write
// anything" assertion below has to look past the response, because the response
// is exactly where the default gets filled in.
func storedSettings(t *testing.T, accountID string) map[string]any {
	t.Helper()
	var raw []byte
	dbfx.QueryRow(t, `SELECT settings FROM content_account WHERE account_id = $1`, accountID).Scan(&raw)
	settings := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &settings); err != nil {
			t.Fatalf("stored settings are not an object: %v", err)
		}
	}
	return settings
}

func readAccount(t *testing.T, wsID, accountID string) map[string]any {
	t.Helper()
	var body map[string]any
	testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", wsID, accountID, "")).Want(http.StatusOK).JSON(&body)
	return body
}

func responseScope(t *testing.T, account map[string]any) string {
	t.Helper()
	settings, ok := account["settings"].(map[string]any)
	if !ok {
		t.Fatalf("response carries no settings object: %v", account["settings"])
	}
	scope, _ := settings[scopeKey].(string)
	return scope
}

// A1 + A2. A brand-new account reads as "all", and reading it leaves the row
// without the key. The two halves belong in one case: filling the default in
// the response is only correct BECAUSE it does not reach storage - an account
// that never chose and one that chose "all" deliberately have to stay
// distinguishable.
func TestAnAccountThatNeverChoseReadsAsAllAndStaysUnwritten(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-default", "owner")
	id := createAccount(t, ws, "zhihu", "默认范围")["account_id"].(string)

	if scope := responseScope(t, readAccount(t, ws, id)); scope != "all" {
		t.Errorf("a new account reads as %q, want \"all\"", scope)
	}
	if _, present := storedSettings(t, id)[scopeKey]; present {
		t.Error("reading the account wrote the default into the row; " +
			"\"never chose\" and \"chose all\" must stay different rows")
	}
}

// The list endpoint fills it too. A start screen that opened from the list
// would otherwise see an empty value on exactly the accounts that never chose.
func TestTheAccountListAlsoCarriesTheDefaultScope(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-default-list", "owner")
	id := createAccount(t, ws, "weibo", "列表默认")["account_id"].(string)

	var body map[string]any
	testutil.Call(t, testHandler.ListContentAccounts, testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)).Want(http.StatusOK).JSON(&body)

	accounts, _ := body["accounts"].([]any)
	if len(accounts) != 1 {
		t.Fatalf("listed %d accounts, want 1", len(accounts))
	}
	if scope := responseScope(t, accounts[0].(map[string]any)); scope != "all" {
		t.Errorf("listed account reads as %q, want \"all\"", scope)
	}
	if _, present := storedSettings(t, id)[scopeKey]; present {
		t.Error("listing accounts wrote the default into the row")
	}
}

// A3. What was set is what comes back, from the row and not just the response.
func TestASetScopeIsStoredAndReadBack(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-set", "owner")
	id := createAccount(t, ws, "douyin", "设置范围")["account_id"].(string)

	for _, scope := range []string{"local", "web", "all"} {
		setScope(t, ws, id, scope).Want(http.StatusOK)

		if got := responseScope(t, readAccount(t, ws, id)); got != scope {
			t.Errorf("after setting %q the account reads as %q", scope, got)
		}
		if got, _ := storedSettings(t, id)[scopeKey].(string); got != scope {
			t.Errorf("after setting %q the row holds %q", scope, got)
		}
	}
}

// A4. The update query replaces the settings blob wholesale, so the endpoint
// merges. Without that, choosing a scope would quietly delete everything else
// the account had.
func TestSettingTheScopeKeepsEveryOtherSetting(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-merge", "owner")
	id := createAccount(t, ws, "bilibili", "合并")["account_id"].(string)
	dbfx.Exec(t, `UPDATE content_account SET settings = '{"something.else":"kept"}'::jsonb WHERE account_id = $1`, id)

	setScope(t, ws, id, "local").Want(http.StatusOK)

	stored := storedSettings(t, id)
	if stored["something.else"] != "kept" {
		t.Errorf("setting the scope destroyed another setting: %v", stored)
	}
	if stored[scopeKey] != "local" {
		t.Errorf("scope = %v", stored[scopeKey])
	}
}

// A5. An unrecognised value is a 400 diagnostic error object, and the row is
// untouched. A half-written settings blob is worse than a refused edit.
func TestAnUnrecognisedScopeIsRefusedAndChangesNothing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-invalid", "owner")
	id := createAccount(t, ws, "kuaishou", "非法值")["account_id"].(string)
	setScope(t, ws, id, "web").Want(http.StatusOK)

	for _, bad := range []string{"everything", "", "All", "LOCAL", "local ", "local,web"} {
		var body map[string]any
		setScope(t, ws, id, bad).Want(http.StatusBadRequest).JSON(&body)

		if body["code"] == nil || body["next_action"] == nil {
			t.Errorf("refusing %q did not return a diagnostic error object: %v", bad, body)
		}
		if got, _ := storedSettings(t, id)[scopeKey].(string); got != "web" {
			t.Errorf("refusing %q changed the stored scope to %q", bad, got)
		}
	}
}

// A6. The dedicated endpoint is not the only way into this key. With only it
// checked, a PATCH could store a value every reader refuses and the endpoint's
// validation would be decoration.
func TestAnUnrecognisedScopeIsAlsoRefusedThroughTheGeneralUpdate(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-patch-invalid", "owner")
	id := createAccount(t, ws, "shipinhao", "PATCH 非法")["account_id"].(string)

	var body map[string]any
	testutil.Call(t, testHandler.UpdateContentAccount,
		accountRequest(t, "PATCH", ws, id, `{"settings":{"loretide.scope":"everything"}}`)).
		Want(http.StatusBadRequest).JSON(&body)

	if body["code"] == nil {
		t.Errorf("PATCH refusal is not a diagnostic error object: %v", body)
	}
	if _, present := storedSettings(t, id)[scopeKey]; present {
		t.Error("a refused PATCH still wrote the scope key")
	}
}

// Creating an account with a bad scope in its settings is refused for the same
// reason: it is a third door into the same key.
func TestAnUnrecognisedScopeIsRefusedAtCreation(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-create-invalid", "owner")

	testutil.Call(t, testHandler.CreateContentAccount, testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-accounts",
			`{"platform":"zhihu","display_name":"建时非法","settings":{"loretide.scope":"nope"}}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)).Want(http.StatusBadRequest)
}

// KNOWN BEHAVIOUR, pinned deliberately (FR-017).
//
// PATCH replaces the settings blob wholesale, so a client that sends only the
// scope key loses everything else. This card does NOT change that - it routes
// its own writes through an endpoint that merges server-side. The case exists
// so that changing this is a decision someone makes, rather than something that
// happens to a test one day.
func TestPatchingOnlyTheScopeStillWipesTheOtherSettings(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-patch-wipes", "owner")
	id := createAccount(t, ws, "zhihu", "整体替换")["account_id"].(string)
	dbfx.Exec(t, `UPDATE content_account SET settings = '{"something.else":"kept"}'::jsonb WHERE account_id = $1`, id)

	testutil.Call(t, testHandler.UpdateContentAccount,
		accountRequest(t, "PATCH", ws, id, `{"settings":{"loretide.scope":"local"}}`)).
		Want(http.StatusOK)

	stored := storedSettings(t, id)
	if _, present := stored["something.else"]; present {
		t.Error("PATCH now merges settings. That may well be an improvement, but it is " +
			"a behaviour change: update specs/018 FR-017 and the PR note together with this test")
	}
	if stored[scopeKey] != "local" {
		t.Errorf("scope = %v", stored[scopeKey])
	}
}

// A7. Accounts do not share a preference.
func TestSettingOneAccountsScopeLeavesAnotherAlone(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-isolation", "owner")
	a := createAccount(t, ws, "zhihu", "A 号")["account_id"].(string)
	b := createAccount(t, ws, "weibo", "B 号")["account_id"].(string)

	setScope(t, ws, b, "web").Want(http.StatusOK)
	for _, scope := range []string{"local", "all", "local"} {
		setScope(t, ws, a, scope).Want(http.StatusOK)
	}

	if got := responseScope(t, readAccount(t, ws, b)); got != "web" {
		t.Errorf("B reads as %q after three changes to A; preferences are per account", got)
	}
}

// A brand-new B stays at the default while A is changed - the same rule from
// the other side, where "unchanged" means "still has no stored value".
func TestChangingOneAccountDoesNotGiveAnotherAStoredValue(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "scope-isolation-default", "owner")
	a := createAccount(t, ws, "zhihu", "A 号")["account_id"].(string)
	b := createAccount(t, ws, "weibo", "B 号")["account_id"].(string)

	setScope(t, ws, a, "local").Want(http.StatusOK)

	if got := responseScope(t, readAccount(t, ws, b)); got != "all" {
		t.Errorf("B reads as %q", got)
	}
	if _, present := storedSettings(t, b)[scopeKey]; present {
		t.Error("changing A gave B a stored preference")
	}
}

// A8. Another brand's account is answered exactly as a missing one.
func TestSettingAnotherBrandsScopeIsIndistinguishableFromAMissingAccount(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	mine := accountWorkspace(t, "scope-mine", "owner")
	theirs := accountWorkspace(t, "scope-theirs", "")
	id := createAccountIn(t, theirs, "zhihu", "别人的账号")

	var refused, missing map[string]any
	setScope(t, mine, id, "local").Want(http.StatusNotFound).JSON(&refused)
	setScope(t, mine, "no-such-account", "local").Want(http.StatusNotFound).JSON(&missing)

	refusedJSON, _ := json.Marshal(refused)
	missingJSON, _ := json.Marshal(missing)
	if string(refusedJSON) != string(missingJSON) {
		t.Errorf("another brand's account answers differently from a missing one:\n%s\n%s",
			refusedJSON, missingJSON)
	}
	if got, _ := storedSettings(t, id)[scopeKey].(string); got != "" {
		t.Errorf("a refused cross-brand write still stored %q", got)
	}
}

// createAccountIn inserts directly, because the caller is not a member of the
// workspace it is inserting into and the endpoint would - correctly - refuse.
func createAccountIn(t *testing.T, wsID, platform, name string) string {
	t.Helper()
	// A literal id: account_id is a text column, and a fixed value makes the
	// cross-brand case readable when it fails.
	id := "cross-brand-" + name
	dbfx.Exec(t, `INSERT INTO content_account (account_id, workspace_id, platform, display_name, settings)
VALUES ($1, $2, $3, $4, '{}'::jsonb)`, id, wsID, platform, name)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM content_account WHERE account_id = $1`, id)
	})
	return id
}
