package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
)

// specs/036 PR 1 without a database: the write path's fence and reference
// answers, over a transaction that records what it was asked (T015, T016,
// T019; FR-013, FR-103, FR-104). The same behaviour against real PostgreSQL
// is in search_theme_integration_test.go and in internal/handler.

// recordingTx is a transaction that answers nothing: any statement it is
// asked to run is counted and fails. Methods it does not override panic,
// which is how a test learns that a path used more than it should.
type recordingTx struct {
	pgx.Tx
	statements int
}

func (tx *recordingTx) QueryRow(context.Context, string, ...any) pgx.Row {
	tx.statements++
	return failingRow{}
}
func (tx *recordingTx) Rollback(context.Context) error { return nil }
func (tx *recordingTx) Commit(context.Context) error   { return errReadFailed }

type recordingDatabase struct {
	failingDatabase
	tx *recordingTx
}

func (d *recordingDatabase) Begin(context.Context) (pgx.Tx, error) { return d.tx, nil }

// allowGuard holds the fence; denyGuard refuses it the way workspace-core
// does once a workspace deletion has committed.
type allowGuard struct{}

func (allowGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error { return nil }

type denyGuard struct{}

func (denyGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	return diagnostics.ErrDenied
}

// stubAccounts answers one account per workspace, or the error it holds.
type stubAccounts struct {
	accounts map[string]ipprofile.Account
	err      error
	reads    int
}

func (a *stubAccounts) Get(_ context.Context, workspaceID, accountID string) (ipprofile.Account, error) {
	a.reads++
	if a.err != nil {
		return ipprofile.Account{}, a.err
	}
	account, ok := a.accounts[workspaceID+"/"+accountID]
	if !ok {
		return ipprofile.Account{}, ipprofile.ErrNotFound
	}
	return account, nil
}

func (a *stubAccounts) CurrentPersonaRevision(context.Context, string, string) (ipprofile.Revision, error) {
	return ipprofile.Revision{}, ipprofile.ErrNotFound
}

// erringSources fails every read.
type erringSources struct{ err error }

func (s erringSources) Exists(context.Context, string, string) (bool, error) { return false, s.err }

func themeStore(guard diagnostics.WorkspaceWriteGuard) (*Store, *recordingTx, *stubAccounts, *fakeSources) {
	tx := &recordingTx{}
	accounts := &stubAccounts{accounts: map[string]ipprofile.Account{
		"ws-a/acct-a": {AccountID: "acct-a", WorkspaceID: "ws-a", Platform: "xiaohongshu"},
		"ws-b/acct-b": {AccountID: "acct-b", WorkspaceID: "ws-b", Platform: "xiaohongshu"},
	}}
	sources := &fakeSources{byWorkspace: map[string][]string{"ws-a": {"src-own"}, "ws-b": {"src-foreign"}}}
	return &Store{
		DB: &recordingDatabase{tx: tx}, Diagnostics: &recordingDiagnostics{}, Guard: guard,
		Accounts: accounts, Sources: sources, Build: "test",
	}, tx, accounts, sources
}

// T019 / FR-103: once the workspace deletion has committed the fence refuses,
// and a theme write answers ErrNotFound - the 404 every fenced write gives,
// not a 503 - without reading an account or a material and without running
// a single statement.
func TestSearchThemeWritesAfterWorkspaceDeletionAreNotFound(t *testing.T) {
	store, tx, accounts, sources := themeStore(denyGuard{})
	content := validTheme()
	content.AccountID, content.SourceIDs = "acct-a", []string{"src-own"}
	_, err := store.CreateSearchTheme(t.Context(), "ws-a", "actor", content)
	if !errors.Is(err, ErrNotFound) || errors.Is(err, ErrStorage) {
		t.Fatalf("create after the deletion = %v, want ErrNotFound", err)
	}
	_, err = store.ReviseSearchTheme(t.Context(), "ws-a", "actor", "theme-1",
		ThemeRevisionRequest{BaseRevision: 1, Content: content})
	if !errors.Is(err, ErrNotFound) || errors.Is(err, ErrStorage) {
		t.Fatalf("revision after the deletion = %v, want ErrNotFound", err)
	}
	if accounts.reads != 0 || sources.reads != 0 || tx.statements != 0 {
		t.Fatalf("after the fence refused: %d account reads, %d material reads, %d statements; want none",
			accounts.reads, sources.reads, tx.statements)
	}
}

// T015 / T016 / FR-013: another brand's material or account is refused
// exactly like one that does not exist, naming the field, before anything
// is written; an account on another platform is refused naming platform.
func TestSearchThemeForeignReferencesAnswerLikeMissingOnes(t *testing.T) {
	for _, tc := range []struct {
		field            string
		foreign, missing func(*ThemeContent)
	}{
		{"source_ids",
			func(c *ThemeContent) { c.SourceIDs = []string{"src-own", "src-foreign"} },
			func(c *ThemeContent) { c.SourceIDs = []string{"src-own", "src-none"} }},
		{"account_id",
			func(c *ThemeContent) { c.AccountID = "acct-b" },
			func(c *ThemeContent) { c.AccountID = "acct-none" }},
	} {
		refusals := []string{}
		for _, edit := range []func(*ThemeContent){tc.foreign, tc.missing} {
			store, tx, _, _ := themeStore(allowGuard{})
			content := validTheme()
			edit(&content)
			_, err := store.CreateSearchTheme(t.Context(), "ws-a", "actor", content)
			wantSearchField(t, err, tc.field)
			if tx.statements != 0 {
				t.Fatalf("%s: %d statements ran for a refused theme", tc.field, tx.statements)
			}
			encoded, _ := json.Marshal(err)
			refusals = append(refusals, string(encoded))
		}
		if refusals[0] != refusals[1] {
			t.Errorf("%s: foreign %s, missing %s - the refusal tells them apart", tc.field, refusals[0], refusals[1])
		}
	}
	store, _, _, _ := themeStore(allowGuard{})
	content := validTheme()
	content.AccountID, content.Platform = "acct-a", "douyin"
	_, err := store.CreateSearchTheme(t.Context(), "ws-a", "actor", content)
	wantSearchField(t, err, "platform")
}

// FR-104, the other direction: a failed account or material read is a
// storage failure, never "not found" and never a 400.
func TestSearchThemeFailedReferenceReadsAreStorage(t *testing.T) {
	store, _, accounts, _ := themeStore(allowGuard{})
	accounts.err = errReadFailed
	content := validTheme()
	content.AccountID = "acct-a"
	if _, err := store.CreateSearchTheme(t.Context(), "ws-a", "actor", content); !errors.Is(err, ErrStorage) {
		t.Fatalf("failed account read = %v, want ErrStorage", err)
	}
	store, _, _, _ = themeStore(allowGuard{})
	store.Sources = erringSources{err: errReadFailed}
	content = validTheme()
	content.SourceIDs = []string{"src-own"}
	if _, err := store.CreateSearchTheme(t.Context(), "ws-a", "actor", content); !errors.Is(err, ErrStorage) {
		t.Fatalf("failed material read = %v, want ErrStorage", err)
	}
	// An adapter that already mapped a missing record to ErrNotFound is
	// still "not here", a 400 naming the field.
	store.Sources = erringSources{err: ErrNotFound}
	_, err := store.CreateSearchTheme(t.Context(), "ws-a", "actor", content)
	wantSearchField(t, err, "source_ids")
}
