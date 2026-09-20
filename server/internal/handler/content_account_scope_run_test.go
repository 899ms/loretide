//go:build dbtest

package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// A10. Running a diagnostic - and reproducing one - must not touch the
// account's stored preference.
//
// The snapshot side of this rule already has an owner:
// diagnostics/reproduce_preference_test.go asserts that reproducing does not
// rewrite Snapshot.Preference (D13-V10, spec 008 G5). That is a different
// STORAGE LOCATION from the account's settings blob, so it says nothing about
// this one, and the two together are what "runs keep using their own field and
// never write the preference back" actually means.
//
// The method is borrowed from that file deliberately: pick a preference the
// fixture never produces. Both simulator.go and shapes.go write "all", which is
// also this feature's default, so asserting on "all" could not tell "nothing
// was written" apart from "something was written and happened to match".
//
// A limitation worth stating rather than glossing: diagnostic runs cannot yet
// be tied to a content account at all. diagnosticScope leaves scope.Accounts
// empty ("Real account permissions are connected when the account domain
// ships"), so a run carrying an account_id is refused. These cases are
// therefore a FORWARD guard - they pass today because runs do not know about
// accounts, not because anything stops a write - and they exist so that
// connecting the two later cannot quietly bring a write-back with it. Mutation
// testing confirmed they are not empty: making the run path write the
// preference to every account in the workspace turns them red.
const nonFixtureAccountScope = "local"

func TestADiagnosticRunDoesNotWriteBackTheAccountsScope(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)

	ws := accountWorkspace(t, "scope-run-writeback", "owner")
	id := createAccount(t, ws, "zhihu", "运行不回写")["account_id"].(string)
	setScope(t, ws, id, nonFixtureAccountScope).Want(http.StatusOK)

	// Reuses the helper specs/008 added rather than a second copy of it.
	run := simulateRun(t, h, ws, "normal", 42, "")
	if run.ID == "" {
		t.Fatal("simulate produced no run")
	}
	assertStoredScope(t, id, nonFixtureAccountScope, "a diagnostic run")

	// The reproduction is the half that historically went wrong: it reads a
	// previous run's recorded inputs, which is exactly the code path that could
	// decide to write one of them back.
	repro := simulateRun(t, h, ws, "normal", 42, run.ID)
	if repro.ID == run.ID {
		t.Fatal("reproducing reused the original run id")
	}
	assertStoredScope(t, id, nonFixtureAccountScope, "reproducing a run")
}

// A run carries its OWN scope on the snapshot, and it is free to differ from
// the account's preference. That independence is the point of having two
// fields: "what this run used" and "what was saved" answer different questions,
// and a run pinned to a fixed scope must not drag the preference along with it.
//
// No new snapshot field is introduced for this; Snapshot.Scope already exists.
func TestARunsOwnScopeIsIndependentOfTheAccountsPreference(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	h := *testHandler
	h.ContentDiagnostics = diagnostics.NewService(h.NewContentDiagnosticsStore(testPool), "test", true)

	ws := accountWorkspace(t, "scope-run-independent", "owner")
	id := createAccount(t, ws, "weibo", "独立字段")["account_id"].(string)
	setScope(t, ws, id, nonFixtureAccountScope).Want(http.StatusOK)

	// Reuses the helper specs/008 added rather than a second copy of it.
	run := simulateRun(t, h, ws, "normal", 42, "")

	if run.Snapshot.Scope == "" {
		t.Error("the run recorded no source scope of its own")
	}
	if run.Snapshot.Scope == nonFixtureAccountScope {
		t.Errorf("the run's source scope came out as the account's preference (%q); "+
			"this case cannot tell the two fields apart when they agree, so if the "+
			"fixture now starts from the account, it needs rewriting deliberately",
			nonFixtureAccountScope)
	}
	assertStoredScope(t, id, nonFixtureAccountScope, "recording a run's own scope")
}

// assertStoredScope reads the row, not a response: the response is where the
// default gets filled in, so it cannot answer "was anything written".
func assertStoredScope(t *testing.T, accountID, want, afterWhat string) {
	t.Helper()
	got, _ := storedSettings(t, accountID)[scopeKey].(string)
	if got != want {
		t.Errorf("%s changed the account's stored scope: %q -> %q", afterWhat, want, got)
	}
}
