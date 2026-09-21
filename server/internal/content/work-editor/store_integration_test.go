package workeditor

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Works, documents and versions against real PostgreSQL.
//
// Contract: specs/024-work-editor-manual/contracts/work-and-versions.md
//
// Runs only with LORETIDE_WORK_TEST_DATABASE_URL set; newWorkFixture skips
// otherwise, and a skip is not a pass.

// workTestGuard stands in for workspace-core's fence. This fixture runs in an
// isolated schema holding the content tables only, so there is no workspace row
// to lock here. The fence itself is proven against the real schema by
// internal/handler's TestWorkEditorWritesAreFencedByWorkspaceDeletion.
type workTestGuard struct{}

func (workTestGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	return nil
}

type workFixture struct {
	store *Store
	db    *testutil.Fixture
	pool  *pgxpool.Pool
}

const (
	testWorkspace = "ws-work"
	testActor     = "actor-work"
	testCard      = "card-1"
)

func newWorkFixture(t *testing.T) workFixture {
	t.Helper()
	url := os.Getenv("LORETIDE_WORK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LORETIDE_WORK_TEST_DATABASE_URL is not set; real PostgreSQL test not run")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "work_test_" + diagnostics.NewID()
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
		"494_content_work.up.sql",
		"495_content_work_id_unique_idx.up.sql",
		"496_content_work_card_idx.up.sql",
		"497_content_artifact.up.sql",
		"498_content_artifact_id_unique_idx.up.sql",
		"499_content_artifact_work_idx.up.sql",
		"500_content_artifact_version.up.sql",
		"501_content_artifact_version_id_unique_idx.up.sql",
		"502_content_artifact_version_unique_idx.up.sql",
		"503_content_artifact_version_workspace_idx.up.sql",
		// specs/031: the historical import column and the fourth action.
		"532_content_work_historical_import.up.sql",
		"535_content_artifact_version_action_imported.up.sql",
		"538_content_import_idempotency.up.sql",
		"539_content_import_idempotency_scope_key_idx.up.sql",
	} {
		sql, readErr := os.ReadFile(filepath.Join(migrations, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return workFixture{
		store: &Store{DB: pool, Diagnostics: diagnostics.NewStore(pool, workTestGuard{}),
			Guard: workTestGuard{}, Build: "test"},
		db:   testutil.New(pool, "", ""),
		pool: pool,
	}
}

// seedArtifact creates a work and one document in it.
func seedArtifact(t *testing.T, fx workFixture) (string, string) {
	t.Helper()
	ctx := t.Context()
	work, err := fx.store.CreateWork(ctx, testWorkspace, testActor, Work{
		TopicCardID: testCard, Title: "第一篇",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	artifact, err := fx.store.CreateArtifact(ctx, testWorkspace, testActor, work.WorkID, Artifact{
		Kind: KindBody, Title: "正文", Position: 1,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	return work.WorkID, artifact.ArtifactID
}

func typeInto(t *testing.T, fx workFixture, workID, artifactID, body string) Artifact {
	t.Helper()
	artifact, err := fx.store.PatchArtifact(t.Context(), testWorkspace, testActor, workID, artifactID,
		ArtifactPatch{DraftBody: &body})
	if err != nil {
		t.Fatalf("autosave: %v", err)
	}
	return artifact
}

func TestAWorkCanBeCreatedWithoutHavingBeenStarted(t *testing.T) {
	// SOP 6.2: writing has to work with nothing else in place. A work with no
	// start snapshot is ordinary, not a half-filled row.
	fx := newWorkFixture(t)
	work, err := fx.store.CreateWork(t.Context(), testWorkspace, testActor, Work{
		TopicCardID: testCard, Title: "无快照",
	})
	if err != nil {
		t.Fatalf("create work without a snapshot: %v", err)
	}
	if work.SnapshotID != "" {
		t.Errorf("snapshot_id = %q, want empty", work.SnapshotID)
	}
	if work.WorkID == "" {
		t.Error("the work has no stable key")
	}
}

// Autosave must never produce a version, and the draft status has to follow.
// Two assertions, not one: if only the count were checked, a status that never
// moved would pass; if only the status were checked, a version written on every
// keystroke would pass.
func TestAutosaveProducesNoVersionAndMovesTheDraftStatus(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)

	for i := range 5 {
		artifact := typeInto(t, fx, workID, artifactID, strings.Repeat("字", i+1))
		if artifact.DraftStatus != DraftWorking {
			t.Fatalf("after autosave %d draft_status = %q, want working", i, artifact.DraftStatus)
		}
	}
	versions, err := fx.store.ListVersions(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("autosave produced %d versions, want 0", len(versions))
	}

	saved, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatalf("save version: %v", err)
	}
	if saved.Source != SourceEdited || saved.Action != ActionSaved {
		t.Errorf("an ordinary save is (%q,%q), want (edited,saved)", saved.Source, saved.Action)
	}
	artifact, err := fx.store.GetArtifact(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.DraftStatus != DraftSaved {
		t.Errorf("after saving a version draft_status = %q, want saved", artifact.DraftStatus)
	}
}

// FR-007b. draft_status is STORED, so it can disagree with what it means. Every
// path that writes it is checked against DraftStatusFor, which is the
// definition.
func TestTheStoredDraftStatusAgreesWithItsDefinition(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)

	assertAgrees := func(t *testing.T, where string) {
		t.Helper()
		artifact, err := fx.store.GetArtifact(ctx, testWorkspace, testActor, workID, artifactID)
		if err != nil {
			t.Fatal(err)
		}
		versions, err := fx.store.ListVersions(ctx, testWorkspace, testActor, workID, artifactID)
		if err != nil {
			t.Fatal(err)
		}
		var latest *ArtifactVersion
		if len(versions) > 0 {
			latest = &versions[0] // newest first
		}
		if want := DraftStatusFor(artifact.DraftBody, latest); artifact.DraftStatus != want {
			t.Errorf("%s: stored draft_status = %q, recomputed = %q", where, artifact.DraftStatus, want)
		}
	}

	assertAgrees(t, "a brand-new document")
	typeInto(t, fx, workID, artifactID, "写了一点")
	assertAgrees(t, "after autosave")
	if _, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID); err != nil {
		t.Fatal(err)
	}
	assertAgrees(t, "after saving a version")

	// Typing back to exactly what the version says really does return the
	// document to saved. A hard-coded "working" would claim unsaved work that
	// does not exist.
	typeInto(t, fx, workID, artifactID, "改了")
	assertAgrees(t, "after typing away from the version")
	typeInto(t, fx, workID, artifactID, "写了一点")
	assertAgrees(t, "after typing back to the version")

	first, err := fx.store.ListVersions(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.RestoreVersion(ctx, testWorkspace, testActor, workID, artifactID,
		first[len(first)-1].VersionID); err != nil {
		t.Fatal(err)
	}
	assertAgrees(t, "after restoring")
}

func TestRestoringAppendsAndLeavesEveryOlderVersionAlone(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)

	bodies := []string{"第一版", "第二版", "第三版"}
	for _, body := range bodies {
		typeInto(t, fx, workID, artifactID, body)
		if _, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID); err != nil {
			t.Fatal(err)
		}
	}
	before, err := fx.store.ListVersions(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	firstVersion := before[len(before)-1]

	restored, err := fx.store.RestoreVersion(ctx, testWorkspace, testActor, workID, artifactID, firstVersion.VersionID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	// Restoring is an ACTION. The content is still what a person wrote, so the
	// source stays edited; "restored" is not a fourth source.
	if restored.Source != SourceEdited {
		t.Errorf("restored version source = %q, want edited", restored.Source)
	}
	if restored.Action != ActionRestored {
		t.Errorf("restored version action = %q, want restored", restored.Action)
	}
	if restored.RestoredFrom != firstVersion.VersionID {
		t.Errorf("restored_from = %q, want %q", restored.RestoredFrom, firstVersion.VersionID)
	}
	if restored.Body != firstVersion.Body {
		t.Errorf("restored body = %q, want %q", restored.Body, firstVersion.Body)
	}

	after, err := fx.store.ListVersions(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("restoring produced %d versions, want %d", len(after), len(before)+1)
	}
	// The history did not rewind: every older version is byte-for-byte what it
	// was, including its source, action and timestamp.
	for i, old := range before {
		if !reflect.DeepEqual(old, after[i+1]) {
			t.Errorf("version %d changed:\n before %+v\n after  %+v", old.Revision, old, after[i+1])
		}
	}
	// And the editor shows what was restored, not the newer text.
	artifact, err := fx.store.GetArtifact(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.DraftBody != firstVersion.Body {
		t.Errorf("the editing copy is %q, want the restored %q", artifact.DraftBody, firstVersion.Body)
	}
}

func TestAdoptingAppendsWithBothColumnsSet(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)
	typeInto(t, fx, workID, artifactID, "候选")
	base, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}

	adopted, err := fx.store.AdoptVersion(ctx, testWorkspace, testActor, workID, artifactID, base.VersionID)
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if adopted.Source != SourceAdopted || adopted.Action != ActionAdopted {
		t.Errorf("adopted version is (%q,%q), want (adopted,adopted)", adopted.Source, adopted.Action)
	}
	if adopted.AdoptedFrom != base.VersionID {
		t.Errorf("adopted_from = %q, want %q", adopted.AdoptedFrom, base.VersionID)
	}
	if adopted.Body != base.Body {
		t.Errorf("adopted body = %q, want a byte-for-byte copy of %q", adopted.Body, base.Body)
	}

	reread, err := fx.store.GetVersion(ctx, testWorkspace, testActor, workID, artifactID, base.VersionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(base, reread) {
		t.Errorf("adopting changed the version it adopted:\n before %+v\n after  %+v", base, reread)
	}
}

// Issue #109's shape, in this module. Numbering inside the locking statement
// picks a revision the winner already used; only concurrency shows it.
func TestConcurrentSavesGetDistinctRevisions(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)
	typeInto(t, fx, workID, artifactID, "并发")

	// No retries. That is the point: a writer that waits for the document's
	// row lock starts its NEXT statement afterwards, and that statement's
	// snapshot includes the winner's row, so it numbers itself correctly the
	// first time. Counting inside the locking statement is what produces a
	// conflict here - the subquery runs against the snapshot taken before the
	// lock was granted, picks a number the winner already used, and turns a
	// legitimate save into an error the caller sees. Allowing retries would
	// hide exactly that, because a retry re-reads and succeeds.
	const writers = 6
	var wg sync.WaitGroup
	revisions := make([]int64, writers)
	failures := make([]error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			version, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID)
			if err != nil {
				failures[i] = err
				return
			}
			revisions[i] = version.Revision
		}()
	}
	wg.Wait()

	seen := map[int64]int{}
	for i, revision := range revisions {
		if failures[i] != nil {
			t.Errorf("writer %d failed with no retry: %v", i, failures[i])
			continue
		}
		seen[revision]++
	}
	for revision, count := range seen {
		if count != 1 {
			t.Errorf("revision %d was handed out %d times", revision, count)
		}
	}
	if len(seen) != writers {
		t.Fatalf("%d writers produced %d distinct revisions", writers, len(seen))
	}
}

func TestAVersionFromAnotherWorkspaceOrWorkIsRefusedAsMissing(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedArtifact(t, fx)
	typeInto(t, fx, workID, artifactID, "内容")
	version, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatal(err)
	}
	otherWorkID, _ := seedArtifact(t, fx)

	for _, tc := range []struct {
		name      string
		workspace string
		workID    string
		versionID string
	}{
		{"another workspace", "ws-other", workID, version.VersionID},
		{"another work", testWorkspace, otherWorkID, version.VersionID},
		{"an unknown version", testWorkspace, workID, "no-such-version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := fx.store.GetVersion(ctx, tc.workspace, testActor, tc.workID, artifactID, tc.versionID); err == nil {
				t.Fatal("the read succeeded")
			}
		})
	}
}

// The same shape as 022's TestBriefStoreHasNoUpdateOrDeletePath and 023's.
// Append-only holds because no second write path exists; the way to keep that
// true is to fail the build when one appears.
//
// The INSERT assertion is not decoration: without it this guard is green on a
// module that writes no SQL at all.
func TestVersionStoreHasNoUpdateOrDeletePath(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	dir := filepath.Dir(current)
	var combined strings.Builder
	for _, name := range []string{"store.go", "artifact.go", "version.go"} {
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		combined.Write(source)
	}
	upper := strings.ToUpper(combined.String())
	for _, forbidden := range []string{
		"UPDATE CONTENT_ARTIFACT_VERSION", "DELETE FROM CONTENT_ARTIFACT_VERSION",
	} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("append-only store contains forbidden path %q", forbidden)
		}
	}
	if !strings.Contains(upper, "INSERT INTO CONTENT_ARTIFACT_VERSION") {
		t.Fatal("append-only store has no insert path")
	}
}

// Constitution IX. The check is on the PATHS, not on the rows: "no generated
// version exists" is trivially true of an empty database.
func TestNothingInThisModuleCanProduceAGeneratedVersion(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		body := string(source)
		// SourceGenerated may be DECLARED (the controlled set has to contain it
		// so EP-08 does not have to widen it later) but must not be USED as a
		// value anywhere a version is built.
		for _, line := range strings.Split(body, "\n") {
			if !strings.Contains(line, "SourceGenerated") {
				continue
			}
			if name == "contract.go" {
				continue
			}
			t.Errorf("%s uses SourceGenerated: %s", name, strings.TrimSpace(line))
		}
	}
}
