package sourceinbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// The material inbox against real PostgreSQL.
//
// Contract: specs/028-source-inbox-manual/contracts/source-inbox.md
//
// Runs only with LORETIDE_SOURCE_TEST_DATABASE_URL set; newSourceFixture skips
// otherwise, and a skip is not a pass.

// sourceTestGuard stands in for workspace-core's fence. This fixture runs in an
// isolated schema holding the content tables only, so there is no workspace row
// to lock here. The fence itself is proven against the real schema by
// internal/handler's fence test.
type sourceTestGuard struct{}

func (sourceTestGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	return nil
}

const (
	testWorkspace = "ws-source"
	testActor     = "actor-source"
)

type sourceFixture struct {
	store *Store
	pool  *pgxpool.Pool
}

func newSourceFixture(t *testing.T) sourceFixture {
	t.Helper()
	url := os.Getenv("LORETIDE_SOURCE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LORETIDE_SOURCE_TEST_DATABASE_URL is not set; real PostgreSQL test not run")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "source_test_" + diagnostics.NewID()
	if _, err = admin.Exec(ctx, `CREATE SCHEMA "`+schema+`"`); err != nil {
		admin.Close()
		t.Fatalf("create isolated schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA "`+schema+`" CASCADE`)
		admin.Close()
	})

	_, current, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(current), "..", "..", "..", "migrations")
	for _, name := range []string{
		"468_content_diagnostics.up.sql",
		"470_content_audit_id.up.sql",
		"471_content_log_id.up.sql",
		"516_content_source.up.sql",
		"519_content_source_snapshot.up.sql",
		"523_content_source_revision.up.sql",
	} {
		sql, readErr := os.ReadFile(filepath.Join(migrations, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return sourceFixture{
		pool: pool,
		store: &Store{
			DB:          pool,
			Diagnostics: diagnostics.NewStore(pool, sourceTestGuard{}),
			Guard:       sourceTestGuard{},
			Build:       "test",
		},
	}
}

func (f sourceFixture) pastedText(t *testing.T, content string) Created {
	t.Helper()
	created, err := f.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindPastedText, Content: content, Title: "note"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	return created
}

func (f sourceFixture) countSnapshots(t *testing.T, sourceID string) int {
	t.Helper()
	var count int
	err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM content_source_snapshot
		WHERE workspace_id=$1 AND source_id=$2`, testWorkspace, sourceID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPastedTextIsStoredWithExactlyOneHashedSnapshot(t *testing.T) {
	fixture := newSourceFixture(t)
	created := fixture.pastedText(t, "something worth keeping")

	if created.Source.Kind != KindPastedText || created.Source.Status != StatusInbox {
		t.Fatalf("created = %+v", created.Source)
	}
	if created.Snapshot == nil {
		t.Fatal("pasted text got no snapshot")
	}
	if created.Snapshot.ContentHash != ContentHash("something worth keeping") {
		t.Errorf("hash = %q, want the hash of the stored body", created.Snapshot.ContentHash)
	}
	if got := fixture.countSnapshots(t, created.Source.SourceID); got != 1 {
		t.Errorf("snapshot count = %d, want exactly 1", got)
	}
}

// FR-002. Counted rather than asserted against a call: "the fetcher was not
// invoked" stops being true the moment the implementation is written another
// way, while "this item has no snapshot" stays the claim.
func TestAURLSourceStoresTheLinkAndNoSnapshot(t *testing.T) {
	fixture := newSourceFixture(t)
	created, err := fixture.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindURL, URL: "https://example.com/post/1", Annotation: "what it says"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	if created.Snapshot != nil {
		t.Error("a url source produced a snapshot; nothing here reads the page")
	}
	if got := fixture.countSnapshots(t, created.Source.SourceID); got != 0 {
		t.Errorf("snapshot count = %d, want 0", got)
	}
	if created.Source.URL != "https://example.com/post/1" {
		t.Errorf("url = %q", created.Source.URL)
	}
	// And no hash to compare against, which is why URL duplicates get no hint.
	if len(created.Duplicates) != 0 {
		t.Errorf("duplicates = %v, want none for a url", created.Duplicates)
	}
}

// R-011: "内容相同不删除独立的收藏上下文与批注".
//
// The assertion that BOTH rows survive is the point. A test that only checked
// "the hint named the older one" would pass just as well against an
// implementation that quietly merged them.
func TestTheSameContentTwiceIsTwoItemsAndOneHint(t *testing.T) {
	fixture := newSourceFixture(t)
	first := fixture.pastedText(t, "the same text")

	second, err := fixture.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindPastedText, Content: "the same text", Annotation: "why I kept it again"})
	if err != nil {
		t.Fatalf("CreateSource: %v", err)
	}
	if len(second.Duplicates) != 1 || second.Duplicates[0] != first.Source.SourceID {
		t.Errorf("duplicates = %v, want [%s]", second.Duplicates, first.Source.SourceID)
	}

	sources, err := fixture.store.ListSources(t.Context(), testWorkspace, testActor, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Fatalf("listed %d sources, want 2; neither collection may be removed", len(sources))
	}
	// Each keeps its own context.
	if first.Source.Annotation == second.Source.Annotation {
		t.Error("the two collections share an annotation")
	}
	if fixture.countSnapshots(t, first.Source.SourceID) != 1 ||
		fixture.countSnapshots(t, second.Source.SourceID) != 1 {
		t.Error("one of the two lost its snapshot")
	}
}

// The duplicate hint reads and returns. It changes nothing.
func TestDuplicatesByHashOnlyReads(t *testing.T) {
	fixture := newSourceFixture(t)
	first := fixture.pastedText(t, "shared")
	second, err := fixture.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindPastedText, Content: "shared"})
	if err != nil {
		t.Fatal(err)
	}

	ids, err := fixture.store.DuplicatesByHash(t.Context(), testWorkspace, testActor, ContentHash("shared"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("DuplicatesByHash returned %v, want both ids", ids)
	}
	sources, err := fixture.store.ListSources(t.Context(), testWorkspace, testActor, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 2 {
		t.Errorf("asking about duplicates changed the inbox: %d sources left", len(sources))
	}
	for _, id := range []string{first.Source.SourceID, second.Source.SourceID} {
		if _, _, err = fixture.store.GetSource(t.Context(), testWorkspace, testActor, id); err != nil {
			t.Errorf("source %s is gone after a duplicate check: %v", id, err)
		}
	}
}

// FR-009 / guard 2, from the data side. The guard reads the SQL; this reads
// the rows, so a path that bypassed the guarded statement would still be seen.
func TestOrganisingNeverChangesTheCollectionFacts(t *testing.T) {
	fixture := newSourceFixture(t)
	created, err := fixture.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindPastedText, Content: "body", Title: "before", HistoricalImport: true})
	if err != nil {
		t.Fatal(err)
	}
	before := created.Source

	title, status := "after", StatusOrganized
	tags := []string{"a", "b"}
	judgement := "kept because it answers a common question"
	for range 5 {
		if _, err = fixture.store.OrganizeSource(t.Context(), testWorkspace, testActor,
			before.SourceID, Organize{Title: &title, Tags: &tags, Status: &status, PersonalJudgement: &judgement}); err != nil {
			t.Fatal(err)
		}
	}

	after, snapshot, err := fixture.store.GetSource(t.Context(), testWorkspace, testActor, before.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Kind != before.Kind || after.URL != before.URL ||
		!after.CapturedAt.Equal(before.CapturedAt) || after.RecordedBy != before.RecordedBy ||
		after.HistoricalImport != before.HistoricalImport {
		t.Errorf("an immutable column changed:\nbefore %+v\nafter  %+v", before, after)
	}
	if snapshot == nil || snapshot.ContentHash != ContentHash("body") {
		t.Error("the snapshot changed under organising")
	}
	if after.Title != "after" || after.Status != StatusOrganized {
		t.Errorf("the mutable half did not change: %+v", after)
	}
}

func TestEveryOrganiseLeavesOneRevision(t *testing.T) {
	fixture := newSourceFixture(t)
	created := fixture.pastedText(t, "body")
	title := "a"
	for _, next := range []string{"a", "b", "c"} {
		title = next
		if _, err := fixture.store.OrganizeSource(t.Context(), testWorkspace, testActor,
			created.Source.SourceID, Organize{Title: &title}); err != nil {
			t.Fatal(err)
		}
	}
	revisions, err := fixture.store.ListRevisions(t.Context(), testWorkspace, testActor, created.Source.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 3 {
		t.Fatalf("got %d revisions, want 3", len(revisions))
	}
	for _, revision := range revisions {
		if len(revision.ChangedFields) != 1 || revision.ChangedFields[0] != "title" {
			t.Errorf("revision named %v, want [title]", revision.ChangedFields)
		}
	}
}

// One row per item, not one per batch. A single "bulk" entry would make "when
// did this item get this tag" unanswerable, which is what the log is for.
func TestBulkOrganiseRecordsEveryItemSeparately(t *testing.T) {
	fixture := newSourceFixture(t)
	first := fixture.pastedText(t, "one")
	second := fixture.pastedText(t, "two")

	archived := StatusArchived
	results, err := fixture.store.BulkOrganize(t.Context(), testWorkspace, testActor,
		[]string{first.Source.SourceID, second.Source.SourceID, "no-such-id"},
		[]string{"reviewed"}, &archived)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want one per requested id", len(results))
	}
	// A batch that half worked says which half: the two that landed stay
	// landed, and the missing one is named.
	if !results[0].OK || !results[1].OK {
		t.Errorf("the two real ids did not succeed: %+v", results)
	}
	if results[2].OK || results[2].Reason != "not_found" {
		t.Errorf("the missing id was not reported: %+v", results[2])
	}
	for _, id := range []string{first.Source.SourceID, second.Source.SourceID} {
		revisions, listErr := fixture.store.ListRevisions(t.Context(), testWorkspace, testActor, id)
		if listErr != nil {
			t.Fatal(listErr)
		}
		if len(revisions) != 1 {
			t.Errorf("source %s has %d revisions, want 1 for its own bulk change", id, len(revisions))
		}
	}
}

// Tags are unioned, not replaced: "add a tag to these" must not drop what each
// already carried.
func TestBulkTagKeepsExistingTags(t *testing.T) {
	fixture := newSourceFixture(t)
	created, err := fixture.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindPastedText, Content: "body", Tags: []string{"kept"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.store.BulkOrganize(t.Context(), testWorkspace, testActor,
		[]string{created.Source.SourceID}, []string{"added"}, nil); err != nil {
		t.Fatal(err)
	}
	after, _, err := fixture.store.GetSource(t.Context(), testWorkspace, testActor, created.Source.SourceID)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, tag := range after.Tags {
		found[tag] = true
	}
	if !found["kept"] || !found["added"] {
		t.Errorf("tags = %v, want both the old and the new", after.Tags)
	}
}

// FR-017: archiving hides an item from a filtered list, and loses nothing.
func TestArchivedItemsAreStillThere(t *testing.T) {
	fixture := newSourceFixture(t)
	created := fixture.pastedText(t, "body")
	archived := StatusArchived
	if _, err := fixture.store.OrganizeSource(t.Context(), testWorkspace, testActor,
		created.Source.SourceID, Organize{Status: &archived}); err != nil {
		t.Fatal(err)
	}
	inbox, err := fixture.store.ListSources(t.Context(), testWorkspace, testActor, string(StatusInbox), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 0 {
		t.Errorf("archived item still listed under inbox: %d", len(inbox))
	}
	stored, err := fixture.store.ListSources(t.Context(), testWorkspace, testActor, string(StatusArchived), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("archived item is not retrievable: %d", len(stored))
	}
	if _, _, err = fixture.store.GetSource(t.Context(), testWorkspace, testActor, created.Source.SourceID); err != nil {
		t.Errorf("archived item cannot be read: %v", err)
	}
}

func TestListFiltersByTag(t *testing.T) {
	fixture := newSourceFixture(t)
	tagged, err := fixture.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindPastedText, Content: "one", Tags: []string{"wanted"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.store.CreateSource(t.Context(), testWorkspace, testActor,
		NewSource{Kind: KindPastedText, Content: "two", Tags: []string{"other"}}); err != nil {
		t.Fatal(err)
	}
	got, err := fixture.store.ListSources(t.Context(), testWorkspace, testActor, "", "wanted")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SourceID != tagged.Source.SourceID {
		t.Errorf("tag filter returned %d rows, want just the tagged one", len(got))
	}
}

// Another brand's id is refused exactly as a missing one.
func TestAnotherWorkspaceCannotReachTheseSources(t *testing.T) {
	fixture := newSourceFixture(t)
	created := fixture.pastedText(t, "body")

	_, _, err := fixture.store.GetSource(t.Context(), "ws-other", testActor, created.Source.SourceID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetSource from another workspace = %v, want ErrNotFound", err)
	}
	title := "x"
	if _, err = fixture.store.OrganizeSource(t.Context(), "ws-other", testActor,
		created.Source.SourceID, Organize{Title: &title}); !errors.Is(err, ErrNotFound) {
		t.Errorf("OrganizeSource from another workspace = %v, want ErrNotFound", err)
	}
	ids, err := fixture.store.DuplicatesByHash(t.Context(), "ws-other", testActor, ContentHash("body"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("another workspace saw duplicates %v", ids)
	}
}

func TestCapturedAtAndRecordedByComeFromTheRequest(t *testing.T) {
	fixture := newSourceFixture(t)
	before := time.Now().UTC().Add(-time.Minute)
	created := fixture.pastedText(t, "body")
	if created.Source.RecordedBy != testActor {
		t.Errorf("recorded_by = %q, want the acting user", created.Source.RecordedBy)
	}
	if created.Source.CapturedAt.Before(before) {
		t.Errorf("captured_at = %v, want roughly now", created.Source.CapturedAt)
	}
}
