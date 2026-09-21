package workeditor

import (
	"testing"
	"time"
)

// SOP 3.3's historical import, against real PostgreSQL.
//
// Contract: specs/031-historical-import/contracts/historical-import.md
//
// Runs only with LORETIDE_WORK_TEST_DATABASE_URL set; newWorkFixture skips
// otherwise, and a skip is not a pass.

// seedImportedArtifact builds what an import produces: a work with no topic
// card, a document, and the pasted body in the editing copy ready to be
// versioned.
func seedImportedArtifact(t *testing.T, fx workFixture, body string) (string, string) {
	t.Helper()
	ctx := t.Context()
	work, err := fx.store.CreateWork(ctx, testWorkspace, testActor, Work{
		Title: "两年前发的那篇", HistoricalImport: true,
	})
	if err != nil {
		t.Fatalf("create imported work: %v", err)
	}
	artifact, err := fx.store.CreateArtifact(ctx, testWorkspace, testActor, work.WorkID, Artifact{
		Kind: KindBody, Title: "正文", Position: 1,
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	typeInto(t, fx, work.WorkID, artifact.ArtifactID, body)
	return work.WorkID, artifact.ArtifactID
}

// A work imported from something already published has no topic card. The
// store used to refuse that outright.
func TestAnImportedWorkNeedsNoTopicCard(t *testing.T) {
	fx := newWorkFixture(t)
	work, err := fx.store.CreateWork(t.Context(), testWorkspace, testActor, Work{
		Title: "历史作品", HistoricalImport: true,
	})
	if err != nil {
		t.Fatalf("create work without a topic card: %v", err)
	}
	if work.TopicCardID != "" {
		t.Errorf("topic_card_id = %q, want empty", work.TopicCardID)
	}
	if !work.HistoricalImport {
		t.Error("historical_import came back false on a work created with it set")
	}
	// It survives a round trip, rather than only existing in the returned
	// struct. The two are different claims and only the second one matters.
	read, err := fx.store.GetWork(t.Context(), testWorkspace, testActor, work.WorkID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !read.HistoricalImport || read.TopicCardID != "" {
		t.Errorf("read back historical_import=%v topic_card_id=%q, want true and empty",
			read.HistoricalImport, read.TopicCardID)
	}
}

// A work created the ordinary way is not marked, and still requires its card
// to be supplied by the caller. Without this the test above would pass against
// a store that set the flag on everything.
func TestAnOrdinaryWorkIsNotMarkedAsImported(t *testing.T) {
	fx := newWorkFixture(t)
	work, err := fx.store.CreateWork(t.Context(), testWorkspace, testActor, Work{
		TopicCardID: testCard, Title: "今天写的",
	})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	if work.HistoricalImport {
		t.Error("an ordinary work came back marked as a historical import")
	}
}

// NEGATIVE TEST - specs/031 FR-034 / FR-035.
//
// This is the one the whole Q3 decision rests on. Allowing topic_card_id=”
// means a work can exist that belongs to no card; if a by-card list matched it,
// an article published two years ago would appear under every topic the
// operator opens, filed as work in progress.
//
// ListWorks' SQL reads ($2=” OR topic_card_id=$2), which happens to exclude it
// because ” never equals a non-empty id. Nobody wrote that expression for this
// case, so it is asserted rather than assumed.
func TestAnImportedWorkIsNotListedUnderAnyTopicCard(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	imported, err := fx.store.CreateWork(ctx, testWorkspace, testActor, Work{
		Title: "历史作品", HistoricalImport: true,
	})
	if err != nil {
		t.Fatalf("create imported work: %v", err)
	}
	onCard, err := fx.store.CreateWork(ctx, testWorkspace, testActor, Work{
		TopicCardID: testCard, Title: "在写的",
	})
	if err != nil {
		t.Fatalf("create ordinary work: %v", err)
	}

	byCard, err := fx.store.ListWorks(ctx, testWorkspace, testActor, testCard)
	if err != nil {
		t.Fatalf("list by card: %v", err)
	}
	for _, work := range byCard {
		if work.WorkID == imported.WorkID {
			t.Fatal("the imported work is listed under a topic card it does not belong to")
		}
	}
	// The positive half. Without it this test is green against a by-card list
	// that returns nothing at all.
	if !containsWork(byCard, onCard.WorkID) {
		t.Fatal("the work that IS on this card is missing; the assertion above proves nothing")
	}
}

// The other half of FR-034: the workspace-wide list is NOT filtered. An
// imported work is a real work here, and hiding it would be a different bug
// from the one above.
func TestTheWorkspaceListStillContainsImportedWorks(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	imported, err := fx.store.CreateWork(ctx, testWorkspace, testActor, Work{
		Title: "历史作品", HistoricalImport: true,
	})
	if err != nil {
		t.Fatalf("create imported work: %v", err)
	}
	all, err := fx.store.ListWorks(ctx, testWorkspace, testActor, "")
	if err != nil {
		t.Fatalf("list workspace works: %v", err)
	}
	if !containsWork(all, imported.WorkID) {
		t.Error("the imported work is missing from the workspace list; only the BY-CARD reading excludes it")
	}
}

func containsWork(works []Work, workID string) bool {
	for _, work := range works {
		if work.WorkID == workID {
			return true
		}
	}
	return false
}

// The import writes exactly one version, and it points at nothing.
//
// "不虚构版本链" is a negative claim, so it is checked as one: restored_from
// and adopted_from empty, and a count of one.
func TestImportingWritesOneVersionWithNoChainBehindIt(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedImportedArtifact(t, fx, "两年前的正文")

	version, err := fx.store.ImportVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatalf("import version: %v", err)
	}
	if version.Source != SourceEdited {
		t.Errorf("source = %q, want %q: the body is still what a person wrote", version.Source, SourceEdited)
	}
	if version.Action != ActionImported {
		t.Errorf("action = %q, want %q", version.Action, ActionImported)
	}
	if version.RestoredFrom != "" || version.AdoptedFrom != "" {
		t.Errorf("restored_from = %q, adopted_from = %q; both must be empty - there is no earlier version to point at",
			version.RestoredFrom, version.AdoptedFrom)
	}
	if version.Body != "两年前的正文" {
		t.Errorf("body = %q, want the pasted text", version.Body)
	}

	versions, err := fx.store.ListVersions(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("the imported document has %d versions, want exactly 1", len(versions))
	}
}

// created_at is the moment of the import, never the historical publication
// date.
//
// The tempting alternative is backdating it so lists sort "correctly". That
// would put a row in the database claiming to be two years old, which the
// audit log written in the same transaction contradicts. The publication date
// lives on the publication record's published_at and nowhere else.
func TestAnImportedVersionIsDatedWhenItWasImported(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedImportedArtifact(t, fx, "正文")

	before := time.Now().UTC().Add(-time.Minute)
	version, err := fx.store.ImportVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatalf("import version: %v", err)
	}
	after := time.Now().UTC().Add(time.Minute)
	if version.CreatedAt.Before(before) || version.CreatedAt.After(after) {
		t.Errorf("created_at = %v, want the moment of the import (between %v and %v)",
			version.CreatedAt, before, after)
	}
}

// An imported document is an ordinary document afterwards: it can be written
// in and saved again. That second version is a save, not an import - nothing
// about being imported is sticky.
func TestAnImportedDocumentCanStillBeEditedAfterwards(t *testing.T) {
	fx := newWorkFixture(t)
	ctx := t.Context()
	workID, artifactID := seedImportedArtifact(t, fx, "原始正文")
	imported, err := fx.store.ImportVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatalf("import version: %v", err)
	}

	typeInto(t, fx, workID, artifactID, "改过的正文")
	saved, err := fx.store.SaveVersion(ctx, testWorkspace, testActor, workID, artifactID)
	if err != nil {
		t.Fatalf("save after import: %v", err)
	}
	if saved.Action != ActionSaved {
		t.Errorf("the second version's action = %q, want %q", saved.Action, ActionSaved)
	}
	if saved.Revision <= imported.Revision {
		t.Errorf("revision %d did not advance past the imported %d", saved.Revision, imported.Revision)
	}
	// The imported version is still there, unchanged. The history is
	// append-only and an import is not a special case of that.
	first, err := fx.store.GetVersion(ctx, testWorkspace, testActor, workID, artifactID, imported.VersionID)
	if err != nil {
		t.Fatalf("read the imported version back: %v", err)
	}
	if first.Body != "原始正文" || first.Action != ActionImported {
		t.Errorf("the imported version changed: body=%q action=%q", first.Body, first.Action)
	}
}
