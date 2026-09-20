//go:build dbtest

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Why this file exists.
//
// A local instance answered 503 "accounts unavailable" and the log held nothing
// but status=503. The cause - a database missing its migrations - took several
// rounds to find, because the response deliberately says nothing and the
// adapter had already flattened the real error into ErrNotFound before any
// handler could record it.
//
// The response must stay exactly as terse as it is: a refusal that explained
// itself would let a caller probe for account ids. So the cause goes to the
// log, with the request id, and never to the client.

// captureLogs swaps in a JSON logger writing to a buffer for the duration of
// one test, and restores the default afterwards.
func captureLogs(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}

// logLines decodes the captured records, skipping anything that is not JSON.
func logLines(t *testing.T, buffer *bytes.Buffer) []map[string]any {
	t.Helper()
	lines := []map[string]any{}
	for _, raw := range strings.Split(buffer.String(), "\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var line map[string]any
		if json.Unmarshal([]byte(raw), &line) != nil {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// A storage failure, not a missing row: the query names a column that is not
// there, so the database refuses and the handler has a real error in hand.
func TestAStorageFailureListingAccountsIsLoggedWithItsCause(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "account-log-list", "owner")
	buffer := captureLogs(t, slog.LevelDebug)

	h := *testHandler
	// Point the handler at a pool with no such table. Any query it runs fails
	// with a real database error rather than "no rows".
	h.Queries = brokenQueries(t)

	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)
	// The membership lookup is the first query the request makes, so with the
	// pool closed that is where it fails. The answer is the ordinary refusal -
	// unchanged, and deliberately indistinguishable from a real non-member -
	// which is exactly why the cause has to reach the log instead.
	testutil.Call(t, h.ListContentAccounts, req).Want(http.StatusNotFound)

	found := false
	for _, line := range logLines(t, buffer) {
		if line["msg"] == "content account request failed" {
			found = true
			if line["error"] == nil || line["error"] == "" {
				t.Error("the log line carries no error text, which is the whole point of it")
			}
		}
	}
	if !found {
		t.Errorf("the request failed with nothing in the log to explain it: %v", logLines(t, buffer))
	}
}

// The response itself must not gain anything. The log is for the operator; the
// caller still learns only that the request failed.
func TestAStorageFailureStillTellsTheClientNothing(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "account-log-body", "owner")
	captureLogs(t, slog.LevelDebug)

	h := *testHandler
	h.Queries = brokenQueries(t)

	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", ws)
	var body map[string]any
	// The membership lookup is the first query the request makes, so with the
	// pool closed that is where it fails. The answer is the ordinary refusal -
	// unchanged, and deliberately indistinguishable from a real non-member -
	// which is exactly why the cause has to reach the log instead.
	testutil.Call(t, h.ListContentAccounts, req).Want(http.StatusNotFound).JSON(&body)

	raw, _ := json.Marshal(body)
	for _, leak := range []string{"content_account", "closed pool", "SQLSTATE", "relation"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("the response leaked storage detail %q: %s", leak, raw)
		}
	}
}

// An ordinary missing account is logged at Debug, not Warn: a 404 for an id
// that was never real is not an operator's problem, and logging every one of
// them at Warn would bury the failures that are.
func TestAMissingAccountIsNotLoggedAsAFailure(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ws := accountWorkspace(t, "account-log-missing", "owner")
	buffer := captureLogs(t, slog.LevelDebug)

	testutil.Call(t, testHandler.GetContentAccount,
		accountRequest(t, "GET", ws, "no-such-account", "")).Want(http.StatusNotFound)

	for _, line := range logLines(t, buffer) {
		if line["msg"] == "content account request failed" {
			t.Errorf("a plain 404 was logged as a failure: %v", line)
		}
	}
}

// brokenQueries returns Queries bound to a pool that has been closed, so every
// query fails with a real database error rather than "no rows". A closed pool
// rather than a bad DSN: it fails instantly and needs no network, and the
// distinction under test is "the database could not answer" versus "the row is
// not there", which either produces.
func brokenQueries(t *testing.T) *db.Queries {
	t.Helper()
	pool, err := pgxpool.NewWithConfig(context.Background(), testPool.Config().Copy())
	if err != nil {
		t.Fatalf("clone pool: %v", err)
	}
	pool.Close()
	return db.New(pool)
}

// A genuine non-member must not produce a warning either. Refusals are ordinary
// and constant; if every one of them logged at Warn, the failures that matter
// would be buried in them — which is the same problem as logging nothing, just
// louder.
func TestANonMemberIsNotLoggedAsAFailure(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	theirs := accountWorkspace(t, "account-log-outsider", "")
	buffer := captureLogs(t, slog.LevelDebug)

	req := testutil.WithHeaders(
		testutil.JSONRequest("GET", "/api/content-accounts", ""),
		"X-User-ID", testUserID, "X-Workspace-ID", theirs)
	testutil.Call(t, testHandler.ListContentAccounts, req).Want(http.StatusNotFound)

	for _, line := range logLines(t, buffer) {
		if line["msg"] == "content account request failed" {
			t.Errorf("a plain non-member refusal was logged as a failure: %v", line)
		}
	}
}
