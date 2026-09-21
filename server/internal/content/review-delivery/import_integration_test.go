package reviewdelivery

import (
	"errors"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// The two columns SOP 3.3's historical import adds to a publication record:
// the direct version pointer (发布后快照) and the import flag.
//
// Contract: specs/031-historical-import/contracts/historical-import.md
//
// Runs only with LORETIDE_REVIEW_TEST_DATABASE_URL set; newReviewFixture skips
// otherwise, and a skip is not a pass.

// recordImport writes what the import path writes: a publication record with
// no delivery task, pointing straight at the version, version_match unknown.
func recordImport(t *testing.T, fx reviewFixture, artifactID, versionID string) PublicationRecord {
	t.Helper()
	record, err := fx.store.Record(t.Context(), testWorkspace, testActor, RecordRequest{
		ArtifactID: artifactID,
		Channel:    ChannelXiaohongshu,
		Status:     PublicationReported,
		DeclaredBy: "导入者本人",
		// 025 already requires this for reported_published. An import is not
		// exempt: without a link there is nothing to go back and check.
		PageURLOrContentID: "https://example.test/note/1",
		VersionID:          versionID,
		HistoricalImport:   true,
	})
	if err != nil {
		t.Fatalf("record imported publication: %v", err)
	}
	return record
}

// A publication with no review request and no delivery task behind it. §3.3:
// the history is recorded, not reconstructed.
func TestAnImportedPublicationNeedsNoDeliveryTask(t *testing.T) {
	fx := newReviewFixture(t)
	artifactID, versionID := seedDraft(t, fx, "body")
	record := recordImport(t, fx, artifactID, versionID)

	if record.DeliveryTaskID != "" {
		t.Errorf("delivery_task_id = %q, want empty: nothing was handed over here", record.DeliveryTaskID)
	}
	if record.VersionID != versionID {
		t.Errorf("version_id = %q, want %q", record.VersionID, versionID)
	}
	if !record.HistoricalImport {
		t.Error("historical_import came back false on an imported record")
	}
	// SOP 9.1's own default. An import never knows whether the platform copy
	// still matches, and unknown is not a failure.
	if record.VersionMatch != VersionUnknown {
		t.Errorf("version_match = %q, want %q", record.VersionMatch, VersionUnknown)
	}

	records, err := fx.store.ListPublications(t.Context(), testWorkspace, testActor, artifactID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].VersionID != versionID || !records[0].HistoricalImport {
		t.Errorf("read back version_id=%q historical_import=%v, want %q and true",
			records[0].VersionID, records[0].HistoricalImport, versionID)
	}
}

// A record made the ordinary way carries neither. Without this the test above
// passes against a store that sets both on everything.
func TestAnOrdinaryPublicationCarriesNeitherNewColumn(t *testing.T) {
	fx := newReviewFixture(t)
	artifactID, _ := seedDraft(t, fx, "body")
	record, err := fx.store.Record(t.Context(), testWorkspace, testActor, RecordRequest{
		ArtifactID: artifactID, Channel: ChannelXiaohongshu,
		Status: PublicationReported, PageURLOrContentID: "https://example.test/note/2",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if record.VersionID != "" {
		t.Errorf("version_id = %q, want empty: this caller stated no version", record.VersionID)
	}
	if record.HistoricalImport {
		t.Error("an ordinary publication came back marked as a historical import")
	}
}

// FR-012. The pointer is to what was published, not to whatever the document
// says now. Two more versions must not move it.
//
// Nothing in the code has to defend this - the table is append-only - which is
// exactly why it is worth an assertion: the day someone adds a "keep the
// snapshot current" convenience, this is what says no.
func TestThePostPublicationSnapshotDoesNotFollowLaterVersions(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	artifactID, versionID := seedDraft(t, fx, "body")
	recordImport(t, fx, artifactID, versionID)

	var workID string
	if err := fx.pool.QueryRow(ctx, `SELECT work_id FROM content_artifact
		WHERE artifact_id=$1`, artifactID).Scan(&workID); err != nil {
		t.Fatal(err)
	}
	for revision := 2; revision <= 3; revision++ {
		saveVersion(t, fx, workID, artifactID, diagnostics.NewID(), revision, "导入之后又改了")
	}

	records, err := fx.store.ListPublications(ctx, testWorkspace, testActor, artifactID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if records[0].VersionID != versionID {
		t.Errorf("version_id moved to %q; the publication is of %q, which is what went out",
			records[0].VersionID, versionID)
	}
}

// 025's own rule, unchanged by the import path: reported_published without a
// link or content id is refused, and the refusal names the field.
//
// Asserted here because the import form is where it will be hit most often,
// and because "the import relaxed it for convenience" is the plausible
// regression.
func TestAnImportedPublicationStillNeedsSomethingToPointAt(t *testing.T) {
	fx := newReviewFixture(t)
	artifactID, versionID := seedDraft(t, fx, "body")
	_, err := fx.store.Record(t.Context(), testWorkspace, testActor, RecordRequest{
		ArtifactID: artifactID, Channel: ChannelXiaohongshu,
		Status: PublicationReported, VersionID: versionID, HistoricalImport: true,
	})
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("err = %v, want a FieldError naming the missing field", err)
	}
	if fieldErr.Field != "page_url_or_content_id" {
		t.Errorf("field = %q, want page_url_or_content_id", fieldErr.Field)
	}
}
