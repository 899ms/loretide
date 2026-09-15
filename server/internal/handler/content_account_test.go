package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Negatives are the point of LT-011, so they are written first and each one is
// named for the situation rather than for the status code it happens to share.
//
// Contract: specs/015-lt011-account-platform-config/contracts/account-api.md

func accountWorkspace(t *testing.T, slug, role string) string {
	t.Helper()
	_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	wsID := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Brand " + slug, "slug": slug, "description": "account test",
	})
	if role != "" {
		dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, $3)`,
			wsID, testUserID, role)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM content_account WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE slug = $1`, slug)
	})
	return wsID
}

func createAccount(t *testing.T, wsID, platform, name string) map[string]any {
	t.Helper()
	req := testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-accounts",
			`{"platform":"`+platform+`","display_name":"`+name+`"}`),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	var body map[string]any
	testutil.Call(t, testHandler.CreateContentAccount, req).Want(http.StatusCreated).JSON(&body)
	return body
}

func accountRequest(t *testing.T, method, wsID, accountID, payload string) *http.Request {
	t.Helper()
	req := testutil.WithHeaders(
		testutil.JSONRequest(method, "/api/content-accounts/"+accountID, payload),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	return withURLParam(req, "id", accountID)
}

// A1: two accounts under one brand do not disturb each other.
func TestTwoAccountsInOneBrandAreIndependent(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "acct-independent", "owner")
	first := createAccount(t, ws, "xiaohongshu", "小红书号")
	second := createAccount(t, ws, "douyin", "抖音号")
	if first["account_id"] == second["account_id"] {
		t.Fatal("two accounts share an id")
	}

	before := map[string]any{}
	testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", ws, second["account_id"].(string), "")).
		Want(http.StatusOK).JSON(&before)

	testutil.Call(t, testHandler.UpdateContentAccount,
		accountRequest(t, "PATCH", ws, first["account_id"].(string),
			`{"display_name":"改过的名字","platform":"bilibili"}`)).
		Want(http.StatusOK)

	after := map[string]any{}
	testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", ws, second["account_id"].(string), "")).
		Want(http.StatusOK).JSON(&after)

	for _, field := range []string{"account_id", "platform", "display_name", "workspace_id"} {
		if before[field] != after[field] {
			t.Errorf("updating one account changed the other's %s: %v -> %v",
				field, before[field], after[field])
		}
	}
}

// A2 and A4 together: a brand you are not in, and a brand that does not exist,
// must be impossible to tell apart. If they differed, the refusal itself would
// be a way to discover which account ids are real.
func TestReadingAnotherBrandsAccountIsIndistinguishableFromOneThatDoesNotExist(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	mine := accountWorkspace(t, "acct-mine", "owner")
	theirs := accountWorkspace(t, "acct-theirs", "") // testUserID is NOT a member
	account := createAccount(t, mine, "zhihu", "我的号")
	accountID := account["account_id"].(string)

	// Same account id, but asked for through a workspace the caller cannot see.
	crossBrand := testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", theirs, accountID, "")).
		Want(http.StatusNotFound).Body.String()

	missing := testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", theirs, "no-such-account-id", "")).
		Want(http.StatusNotFound).Body.String()

	if crossBrand != missing {
		t.Errorf("a real account in another brand and a missing one answer differently, "+
			"which tells a caller which ids exist:\n  other brand: %s\n  missing:     %s",
			crossBrand, missing)
	}
	for _, leak := range []string{"我的号", "zhihu", accountID, "display_name"} {
		if stringContains(crossBrand, leak) {
			t.Errorf("refusal leaked %q: %s", leak, crossBrand)
		}
	}
}

// A3: a cross-brand write is refused AND changes nothing.
func TestWritingToAnotherBrandsAccountIsRefusedAndChangesNothing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	mine := accountWorkspace(t, "acct-write-mine", "owner")
	theirs := accountWorkspace(t, "acct-write-theirs", "")
	account := createAccount(t, mine, "weibo", "原名")
	accountID := account["account_id"].(string)

	testutil.Call(t, testHandler.UpdateContentAccount,
		accountRequest(t, "PATCH", theirs, accountID, `{"display_name":"被改了"}`)).
		Want(http.StatusNotFound)

	var after map[string]any
	testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", mine, accountID, "")).
		Want(http.StatusOK).JSON(&after)
	if after["display_name"] != "原名" {
		t.Errorf("a refused cross-brand write still changed the name to %v", after["display_name"])
	}
}

// A5: an unsupported platform is a 400 diagnostic error object, not a database
// constraint error, and nothing is written.
func TestAnUnsupportedPlatformIsRefusedWith400(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "acct-bad-platform", "owner")
	req := testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-accounts",
			`{"platform":"myspace","display_name":"不该存在"}`),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)
	rec := testutil.Call(t, testHandler.CreateContentAccount, req).Want(http.StatusBadRequest)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v: %s", err, rec.Body.String())
	}
	if body["code"] == nil {
		t.Errorf("the refusal is not a diagnostic error object: %v", body)
	}

	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account WHERE workspace_id = $1`, ws).Scan(&count)
	if count != 0 {
		t.Errorf("a rejected platform still created %d account(s)", count)
	}
}

// A9: the list is scoped and stably ordered.
func TestListingReturnsOnlyThisBrandsAccountsInAStableOrder(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	mine := accountWorkspace(t, "acct-list-mine", "owner")
	other := accountWorkspace(t, "acct-list-other", "owner")
	createAccount(t, mine, "xiaohongshu", "甲")
	createAccount(t, mine, "douyin", "乙")
	createAccount(t, other, "zhihu", "别人的")

	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", mine)
	var first map[string]any
	testutil.Call(t, testHandler.ListContentAccounts, req).Want(http.StatusOK).JSON(&first)
	accounts, _ := first["accounts"].([]any)
	if len(accounts) != 2 {
		t.Fatalf("listing returned %d accounts, want 2 (the other brand's must not appear)", len(accounts))
	}
	for _, raw := range accounts {
		item := raw.(map[string]any)
		if item["workspace_id"] != mine {
			t.Errorf("listing returned an account from workspace %v", item["workspace_id"])
		}
	}

	var second map[string]any
	testutil.Call(t, testHandler.ListContentAccounts, req).Want(http.StatusOK).JSON(&second)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(second)
	if string(a) != string(b) {
		t.Errorf("two identical list requests returned different orders:\n %s\n %s", a, b)
	}
}

// A10 / US4: deleting the brand takes its accounts with it. This is the pair
// #45 missed - the manifest entry alone would leave the rows behind.
func TestDeletingAWorkspaceRemovesItsAccounts(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "acct-deleted-brand", "owner")
	createAccount(t, ws, "kuaishou", "要被删的号")

	var before int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account WHERE workspace_id = $1`, ws).Scan(&before)
	if before == 0 {
		t.Fatal("setup: expected an account to exist")
	}

	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest("DELETE", "/api/workspaces/"+ws, ""),
		"X-User-ID", testUserID), "id", ws)
	testutil.Call(t, testHandler.DeleteWorkspace, req).WantOneOf(http.StatusNoContent, http.StatusOK)

	var after int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account WHERE workspace_id = $1`, ws).Scan(&after)
	if after != 0 {
		t.Errorf("%d account(s) survived the workspace deletion", after)
	}
}

// The case the task card names first: ONE user who belongs to both brands.
//
// The cross-brand tests above are refused by authorization before any query
// runs, so they prove the guard works but say nothing about whether the queries
// themselves are workspace-scoped. A member of both brands passes the guard for
// B and can then hand over an account id from A - and only the WHERE clause
// stands between that and reading or rewriting another brand's account.
//
// This gap was found by mutation testing: removing workspace_id from the UPDATE
// left every test green.
func TestAMemberOfTwoBrandsCannotReachOneBrandsAccountWhileActingInTheOther(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	brandA := accountWorkspace(t, "acct-both-a", "owner")
	brandB := accountWorkspace(t, "acct-both-b", "owner") // same user owns both
	account := createAccount(t, brandA, "bilibili", "A 的号")
	accountID := account["account_id"].(string)

	// Reading A's account while acting in B: refused, and the refusal is the
	// same one a missing account gets.
	inB := testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", brandB, accountID, "")).
		Want(http.StatusNotFound).Body.String()
	missing := testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", brandB, "no-such-account", "")).
		Want(http.StatusNotFound).Body.String()
	if inB != missing {
		t.Errorf("reading another brand's account and a missing one answer differently:\n %s\n %s",
			inB, missing)
	}

	// Writing to A's account while acting in B: refused, and A is untouched.
	testutil.Call(t, testHandler.UpdateContentAccount,
		accountRequest(t, "PATCH", brandB, accountID, `{"display_name":"越权改的"}`)).
		Want(http.StatusNotFound)

	var after map[string]any
	testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", brandA, accountID, "")).
		Want(http.StatusOK).JSON(&after)
	if after["display_name"] != "A 的号" {
		t.Errorf("a member of both brands rewrote brand A's account from brand B: name is now %v",
			after["display_name"])
	}
	if after["workspace_id"] != brandA {
		t.Errorf("the account moved to workspace %v", after["workspace_id"])
	}
}
