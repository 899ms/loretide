package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// The bug these cases exist for.
//
// accountIDFromURL read the id through workspaceIDFromURL, which prefers
// middleware.WorkspaceIDFromContext over the chi URL parameter. Behind the real
// router that context is always populated, so every account endpoint with an
// {id} in its path received the WORKSPACE id as the account id and answered 404
// for accounts that plainly existed.
//
// Nothing caught it, and the reason is worth stating: a handler test calls the
// function directly, so the workspace context is empty and the fallback - the
// URL parameter - is what gets read. The handler tests in #71, #73 and #79
// therefore exercised the one path production never takes. #79's route cases
// proved the URLs were mounted, but sent ids that never had to survive the
// middleware.
//
// So these go through testServer: the real router, the real workspace
// middleware, and an account id that is deliberately NOT the workspace id.
// Every assertion below fails on the old code.

func accountAPIRequest(t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, testServer.URL+path, bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-ID", testWorkspaceID)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return response
}

// createAccountThroughTheAPI returns an id the server chose, so the test cannot
// accidentally use a value shaped like a workspace id.
func createAccountThroughTheAPI(t *testing.T, displayName string) string {
	t.Helper()
	response := accountAPIRequest(t, http.MethodPost, "/api/content-accounts",
		fmt.Sprintf(`{"platform":"zhihu","display_name":%q}`, displayName))
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create account = %d, want 201", response.StatusCode)
	}
	var account struct {
		AccountID string `json:"account_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&account); err != nil {
		t.Fatalf("decode account: %v", err)
	}
	if account.AccountID == "" {
		t.Fatal("created account has no id")
	}
	if account.AccountID == testWorkspaceID {
		t.Fatal("the account id equals the workspace id, so this test could not tell them apart")
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM content_account_revision WHERE account_id = $1`, account.AccountID)
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM content_account WHERE account_id = $1`, account.AccountID)
	})
	return account.AccountID
}

func TestAccountEndpointsUseTheIdFromTheURLBehindTheRealRouter(t *testing.T) {
	if testServer == nil {
		t.Skip("database not available")
	}
	accountID := createAccountThroughTheAPI(t,
		fmt.Sprintf("URL id account %d", time.Now().UnixNano()))

	t.Run("GET returns the account named in the path", func(t *testing.T) {
		response := accountAPIRequest(t, http.MethodGet, "/api/content-accounts/"+accountID, "")
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET account = %d, want 200 — the id in the path was not the id that was looked up",
				response.StatusCode)
		}
		var account struct {
			AccountID string `json:"account_id"`
		}
		if err := json.NewDecoder(response.Body).Decode(&account); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if account.AccountID != accountID {
			t.Errorf("GET %s returned account %q", accountID, account.AccountID)
		}
	})

	t.Run("PATCH edits the account named in the path", func(t *testing.T) {
		response := accountAPIRequest(t, http.MethodPatch, "/api/content-accounts/"+accountID,
			`{"display_name":"改过的名字"}`)
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("PATCH account = %d, want 200", response.StatusCode)
		}
		var stored string
		if err := testPool.QueryRow(context.Background(),
			`SELECT display_name FROM content_account WHERE account_id = $1`, accountID).
			Scan(&stored); err != nil {
			t.Fatalf("read back: %v", err)
		}
		if stored != "改过的名字" {
			t.Errorf("stored display name = %q", stored)
		}
	})
}

// The persona endpoints are the ones the browser actually hit. A 404 on GET
// alone would be ambiguous - an account with no revision answers 404 too - so
// the write comes first: POST must be 201, and only then does GET's 200 mean
// the id was read from the path.
func TestPersonaEndpointsUseTheIdFromTheURLBehindTheRealRouter(t *testing.T) {
	if testServer == nil {
		t.Skip("database not available")
	}
	accountID := createAccountThroughTheAPI(t,
		fmt.Sprintf("URL id persona %d", time.Now().UnixNano()))

	write := accountAPIRequest(t, http.MethodPost, "/api/content-accounts/"+accountID+"/persona",
		`{"persona_prompt":"来自真实路由的人设"}`)
	defer write.Body.Close()
	if write.StatusCode != http.StatusCreated {
		t.Fatalf("POST persona = %d, want 201 — this is the 404 a real browser saw", write.StatusCode)
	}

	read := accountAPIRequest(t, http.MethodGet, "/api/content-accounts/"+accountID+"/persona", "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		t.Fatalf("GET persona = %d, want 200", read.StatusCode)
	}
	var revision struct {
		AccountID     string `json:"account_id"`
		PersonaPrompt string `json:"persona_prompt"`
	}
	if err := json.NewDecoder(read.Body).Decode(&revision); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if revision.AccountID != accountID {
		t.Errorf("GET persona for %q returned account %q", accountID, revision.AccountID)
	}
	if revision.PersonaPrompt != "来自真实路由的人设" {
		t.Errorf("persona prompt = %q", revision.PersonaPrompt)
	}

	list := accountAPIRequest(t, http.MethodGet,
		"/api/content-accounts/"+accountID+"/persona/revisions", "")
	defer list.Body.Close()
	if list.StatusCode != http.StatusOK {
		t.Errorf("GET persona revisions = %d, want 200", list.StatusCode)
	}
}

// The expression-profile endpoints carry the same path parameter through the
// real workspace middleware. The account id deliberately differs from the
// workspace id, so borrowing workspaceIDFromURL here would reproduce the
// production-only 404 that direct handler calls cannot see.
func TestExpressionProfileEndpointsUseTheIdFromTheURLBehindTheRealRouter(t *testing.T) {
	if testServer == nil {
		t.Skip("database not available")
	}
	accountID := createAccountThroughTheAPI(t,
		fmt.Sprintf("URL id profile %d", time.Now().UnixNano()))

	write := accountAPIRequest(t, http.MethodPost,
		"/api/content-accounts/"+accountID+"/profile", `{
			"audience":{"value":"designers","status":"confirmed"},
			"content_pillars":{"value":"tools","status":"confirmed"},
			"primary_channels":{"values":["zhihu"],"status":"confirmed"},
			"weekly_hours":{"value":4,"status":"confirmed"}
		}`)
	defer write.Body.Close()
	if write.StatusCode != http.StatusCreated {
		t.Fatalf("POST profile = %d, want 201", write.StatusCode)
	}

	read := accountAPIRequest(t, http.MethodGet,
		"/api/content-accounts/"+accountID+"/profile", "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		t.Fatalf("GET profile = %d, want 200", read.StatusCode)
	}
	var response struct {
		Profile struct {
			Audience struct {
				Value string `json:"value"`
			} `json:"audience"`
		} `json:"profile"`
		Readiness struct {
			CanStart bool `json:"can_start"`
		} `json:"readiness"`
	}
	if err := json.NewDecoder(read.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.Profile.Audience.Value != "designers" || !response.Readiness.CanStart {
		t.Fatalf("profile response = %+v", response)
	}
}

// The scope endpoint is held to the same standard as the persona ones, and for
// the same reason: it reads the account id out of the path through the very
// helper that used to return the workspace id instead. A route-existence case
// would not catch a repeat - it proves the URL is mounted, not that the id
// survives the middleware - so this one goes through the real router with an
// account id that is deliberately not the workspace id, and reads the row back
// afterwards rather than trusting the status code.
func TestScopeEndpointUsesTheIdFromTheURLBehindTheRealRouter(t *testing.T) {
	if testServer == nil {
		t.Skip("database not available")
	}
	accountID := createAccountThroughTheAPI(t,
		fmt.Sprintf("URL id scope %d", time.Now().UnixNano()))

	write := accountAPIRequest(t, http.MethodPut,
		"/api/content-accounts/"+accountID+"/scope", `{"scope":"local"}`)
	defer write.Body.Close()
	if write.StatusCode != http.StatusOK {
		t.Fatalf("PUT scope = %d, want 200", write.StatusCode)
	}

	var stored string
	if err := testPool.QueryRow(context.Background(),
		`SELECT settings->>'loretide.scope' FROM content_account WHERE account_id = $1`, accountID).
		Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored != "local" {
		t.Errorf("stored scope = %q, want \"local\" — the row the URL named is not the row that was written", stored)
	}

	// And the account reads back through the same path, default filled in the
	// response rather than in the row.
	read := accountAPIRequest(t, http.MethodGet, "/api/content-accounts/"+accountID, "")
	defer read.Body.Close()
	if read.StatusCode != http.StatusOK {
		t.Fatalf("GET account = %d, want 200", read.StatusCode)
	}
	var account struct {
		AccountID string         `json:"account_id"`
		Settings  map[string]any `json:"settings"`
	}
	if err := json.NewDecoder(read.Body).Decode(&account); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if account.AccountID != accountID {
		t.Errorf("GET returned account %q", account.AccountID)
	}
	if account.Settings["loretide.scope"] != "local" {
		t.Errorf("response scope = %v", account.Settings["loretide.scope"])
	}
}

// A fresh account behind the real router reads as the default, and the row
// still holds nothing. The equivalent handler-level case cannot see a
// middleware mistake; this one can.
func TestAFreshAccountReadsAsAllBehindTheRealRouter(t *testing.T) {
	if testServer == nil {
		t.Skip("database not available")
	}
	accountID := createAccountThroughTheAPI(t,
		fmt.Sprintf("URL id default scope %d", time.Now().UnixNano()))

	read := accountAPIRequest(t, http.MethodGet, "/api/content-accounts/"+accountID, "")
	defer read.Body.Close()
	var account struct {
		Settings map[string]any `json:"settings"`
	}
	if err := json.NewDecoder(read.Body).Decode(&account); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if account.Settings["loretide.scope"] != "all" {
		t.Errorf("a fresh account reads as %v, want \"all\"", account.Settings["loretide.scope"])
	}

	var stored *string
	if err := testPool.QueryRow(context.Background(),
		`SELECT settings->>'loretide.scope' FROM content_account WHERE account_id = $1`, accountID).
		Scan(&stored); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if stored != nil {
		t.Errorf("reading the account wrote %q into the row", *stored)
	}
}
