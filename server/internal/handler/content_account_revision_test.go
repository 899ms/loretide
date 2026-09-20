//go:build dbtest

package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// Contract: specs/016-lt012-persona-prompt-revisions/contracts/persona-revision.md

func setPrompt(t *testing.T, wsID, accountID, prompt string) map[string]any {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"persona_prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest("POST", "/api/content-accounts/"+accountID+"/persona", string(payload)),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID), "id", accountID)
	var body map[string]any
	testutil.Call(t, testHandler.SetAccountPersonaPrompt, req).Want(http.StatusCreated).JSON(&body)
	return body
}

func revisionRequest(t *testing.T, wsID, accountID, revisionID string) *http.Request {
	t.Helper()
	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts/"+accountID+"/persona/"+revisionID, ""),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	// withURLParams (plural) preserves what is already on the request; the
	// singular form builds a fresh route context and would drop "id".
	return withURLParams(req, "id", accountID, "revisionId", revisionID)
}

// A1: each confirmation appends. The earlier revision is not rewritten.
func TestEachConfirmationAppendsANewRevision(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "rev-append", "owner")
	account := createAccount(t, ws, "xiaohongshu", "号")
	id := account["account_id"].(string)

	first := setPrompt(t, ws, id, "第一版人设")
	second := setPrompt(t, ws, id, "第二版人设")

	if first["revision"].(float64) != 1 || second["revision"].(float64) != 2 {
		t.Fatalf("revisions are %v and %v, want 1 and 2", first["revision"], second["revision"])
	}
	if first["revision_id"] == second["revision_id"] {
		t.Fatal("both revisions share an id")
	}

	// A5: the first revision still reads as it was written.
	var reread map[string]any
	testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, ws, id, first["revision_id"].(string))).
		Want(http.StatusOK).JSON(&reread)
	if reread["persona_prompt"] != "第一版人设" {
		t.Errorf("the first revision now reads %q; a pinned revision must never change",
			reread["persona_prompt"])
	}
}

// A2: blank is a real revision, and it reads back as an empty string rather
// than null or an error. SOP 3.1's "unconfirmed items remain pending".
func TestABlankPromptStillProducesAReadableRevision(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "rev-blank", "owner")
	account := createAccount(t, ws, "douyin", "号")
	id := account["account_id"].(string)

	created := setPrompt(t, ws, id, "")
	if created["revision"].(float64) != 1 {
		t.Fatalf("a blank prompt did not produce revision 1: %v", created)
	}
	prompt, ok := created["persona_prompt"]
	if !ok || prompt != "" {
		t.Errorf("blank prompt came back as %v (%T), want an empty string - not null, not absent",
			prompt, prompt)
	}

	// And it stays blank once a later revision fills it in.
	setPrompt(t, ws, id, "后来补的人设")
	var reread map[string]any
	testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, ws, id, created["revision_id"].(string))).
		Want(http.StatusOK).JSON(&reread)
	if reread["persona_prompt"] != "" {
		t.Errorf("the blank revision now reads %q", reread["persona_prompt"])
	}
}

// A4: revisions count per account. A global sequence would make B's version
// number jump whenever A was edited.
func TestEditingOneAccountDoesNotTouchAnother(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "rev-isolation", "owner")
	a := createAccount(t, ws, "zhihu", "A 号")["account_id"].(string)
	b := createAccount(t, ws, "weibo", "B 号")["account_id"].(string)

	setPrompt(t, ws, b, "B 的旧人设")
	for _, prompt := range []string{"A1", "A2", "A3"} {
		setPrompt(t, ws, a, prompt)
	}
	// B is written again AFTER A's edits, and this is the point of the test.
	// Asserting only on a B written before them passes even if the counter is
	// global, because B would still hold the number it was given first.
	// Mutation testing is what showed that.
	setPrompt(t, ws, b, "B 的人设")

	var current map[string]any
	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts/"+b+"/persona", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", ws), "id", b)
	testutil.Call(t, testHandler.GetAccountPersonaPrompt, req).Want(http.StatusOK).JSON(&current)

	if current["revision"].(float64) != 2 {
		t.Errorf("B is at revision %v after its own second edit with three edits to A in between; "+
			"B's second revision is 2 and revisions must count per account, not per workspace",
			current["revision"])
	}
	if current["persona_prompt"] != "B 的人设" {
		t.Errorf("B's prompt changed to %q", current["persona_prompt"])
	}
}

// A7. Real concurrency: two goroutines writing at once. Writing twice in
// sequence would pass without the unique index existing at all, so it would
// test nothing.
func TestTwoConcurrentConfirmationsBothLandWithDistinctNumbers(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "rev-concurrent", "owner")
	id := createAccount(t, ws, "bilibili", "号")["account_id"].(string)

	var wg sync.WaitGroup
	results := make([]map[string]any, 2)
	codes := make([]int, 2)
	start := make(chan struct{})
	for i := range results {
		wg.Add(1)
		go func(slot int) {
			defer wg.Done()
			payload, _ := json.Marshal(map[string]string{
				"persona_prompt": []string{"甲写的", "乙写的"}[slot],
			})
			req := withURLParam(testutil.WithHeaders(
				testutil.JSONRequest("POST", "/api/content-accounts/"+id+"/persona", string(payload)),
				"X-User-ID", testUserID, "X-Workspace-ID", ws), "id", id)
			<-start // release both at the same moment
			// testutil.Call only runs the handler and captures the recorder; the
			// assertions that touch t live on Response.Want, which is not called
			// here because Fatalf off the main goroutine is undefined.
			rec := testutil.Call(t, testHandler.SetAccountPersonaPrompt, req)
			codes[slot] = rec.Code
			var body map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			results[slot] = body
		}(i)
	}
	close(start)
	wg.Wait()

	for slot, code := range codes {
		if code != http.StatusCreated {
			t.Fatalf("writer %d got %d, want 201 - a losing writer must retry and land, "+
				"not lose its confirmation: %v", slot, code, results[slot])
		}
	}
	first, second := results[0]["revision"].(float64), results[1]["revision"].(float64)
	if first == second {
		t.Errorf("both writers claimed revision %v; the unique index should have forced a retry", first)
	}
	if (first != 1 || second != 2) && (first != 2 || second != 1) {
		t.Errorf("revisions are %v and %v, want 1 and 2", first, second)
	}

	// And exactly two rows exist - nothing was lost and nothing duplicated.
	var count int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account_revision WHERE account_id = $1`, id).Scan(&count)
	if count != 2 {
		t.Errorf("%d revisions stored, want 2", count)
	}
}

// A9: another brand's revision and a revision that does not exist answer the
// same way, or the refusal itself reveals which ids are real.
func TestAnotherBrandsRevisionIsIndistinguishableFromAMissingOne(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	mine := accountWorkspace(t, "rev-mine", "owner")
	theirs := accountWorkspace(t, "rev-theirs", "")
	id := createAccount(t, mine, "kuaishou", "我的号")["account_id"].(string)
	revision := setPrompt(t, mine, id, "我的人设")["revision_id"].(string)

	hidden := testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, theirs, id, revision)).Want(http.StatusNotFound).Body.String()
	missing := testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, theirs, id, "no-such-revision")).Want(http.StatusNotFound).Body.String()

	if hidden != missing {
		t.Errorf("a real revision in another brand and a missing one answer differently:\n %s\n %s",
			hidden, missing)
	}
	for _, leak := range []string{"我的人设", revision, "persona_prompt"} {
		if stringContains(hidden, leak) {
			t.Errorf("refusal leaked %q: %s", leak, hidden)
		}
	}
}

// A member of both brands passes authorization for the wrong one and hands over
// an id from the other - the case that only the query's own scoping catches.
func TestAMemberOfTwoBrandsCannotReadTheOthersRevision(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	brandA := accountWorkspace(t, "rev-both-a", "owner")
	brandB := accountWorkspace(t, "rev-both-b", "owner")
	id := createAccount(t, brandA, "shipinhao", "A 号")["account_id"].(string)
	revision := setPrompt(t, brandA, id, "A 的人设")["revision_id"].(string)

	testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, brandB, id, revision)).Want(http.StatusNotFound)
}

// A11: revisions go with the workspace.
func TestDeletingAWorkspaceRemovesItsRevisions(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "rev-deleted-brand", "owner")
	id := createAccount(t, ws, "wechat_mp", "号")["account_id"].(string)
	setPrompt(t, ws, id, "要被删的人设")

	var before int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account_revision WHERE workspace_id = $1`, ws).Scan(&before)
	if before == 0 {
		t.Fatal("setup: expected a revision to exist")
	}

	req := withURLParam(testutil.WithHeaders(
		testutil.JSONRequest("DELETE", "/api/workspaces/"+ws, ""),
		"X-User-ID", testUserID), "id", ws)
	testutil.Call(t, testHandler.DeleteWorkspace, req).WantOneOf(http.StatusNoContent, http.StatusOK)

	var after int
	dbfx.QueryRow(t, `SELECT count(*) FROM content_account_revision WHERE workspace_id = $1`, ws).Scan(&after)
	if after != 0 {
		t.Errorf("%d revision(s) survived the workspace deletion", after)
	}
}

// The gap mutation testing found: the cross-brand tests are stopped by the
// query's workspace filter before the account check ever runs, so removing that
// check changed nothing. Two accounts in the SAME workspace is what exercises
// it - a revision id from account A, asked for through account B.
func TestARevisionCannotBeReadThroughADifferentAccountInTheSameBrand(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "rev-wrong-account", "owner")
	a := createAccount(t, ws, "xiaohongshu", "A 号")["account_id"].(string)
	b := createAccount(t, ws, "douyin", "B 号")["account_id"].(string)
	revision := setPrompt(t, ws, a, "A 的人设")["revision_id"].(string)

	// Same workspace, so the workspace filter lets it through; only the account
	// check stands between B and A's persona.
	throughB := testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, ws, b, revision)).Want(http.StatusNotFound).Body.String()
	missing := testutil.Call(t, testHandler.GetAccountPersonaRevision,
		revisionRequest(t, ws, b, "no-such-revision")).Want(http.StatusNotFound).Body.String()

	if throughB != missing {
		t.Errorf("reading A's revision through B answers differently from a missing one:\n %s\n %s",
			throughB, missing)
	}
	if stringContains(throughB, "A 的人设") {
		t.Errorf("the refusal leaked A's persona: %s", throughB)
	}
}
