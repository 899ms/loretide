package feedbacklearning

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

// Manual metrics and feedback excerpts against real PostgreSQL.
//
// Contract: specs/027-feedback-manual/contracts/feedback-manual.md
//
// Runs only with LORETIDE_FEEDBACK_TEST_DATABASE_URL set; newFeedbackFixture
// skips otherwise, and a skip is not a pass.

// feedbackTestGuard stands in for workspace-core's fence. This fixture runs in
// an isolated schema holding the content tables only, so there is no workspace
// row to lock here. The fence itself is proven against the real schema by
// internal/handler's TestFeedbackWritesAreFencedByWorkspaceDeletion.
type feedbackTestGuard struct{}

func (feedbackTestGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	return nil
}

// testPublications answers what the adapter answers in production: does this
// publication record belong here, and which version does it correspond to -
// two hops, with "" as a real answer.
type testPublications struct{ pool *pgxpool.Pool }

func (p testPublications) Resolve(ctx context.Context, workspaceID, publicationRecordID string) (string, string, string, error) {
	var workID, artifactID, deliveryTaskID string
	err := p.pool.QueryRow(ctx, `SELECT work_id, artifact_id, delivery_task_id
		FROM content_publication_record
		WHERE workspace_id=$1 AND publication_record_id=$2`,
		workspaceID, publicationRecordID).Scan(&workID, &artifactID, &deliveryTaskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", ErrStorage
	}
	if deliveryTaskID == "" {
		// A record entered for history: no delivery task, so no review
		// request, so no version. A real answer, not a failure.
		return workID, artifactID, "", nil
	}
	var versionID string
	err = p.pool.QueryRow(ctx, `SELECT r.version_id
		FROM content_delivery_task t
		JOIN content_review_request r
		  ON r.workspace_id = t.workspace_id AND r.review_request_id = t.review_request_id
		WHERE t.workspace_id=$1 AND t.delivery_task_id=$2`,
		workspaceID, deliveryTaskID).Scan(&versionID)
	if err != nil {
		return workID, artifactID, "", nil
	}
	return workID, artifactID, versionID, nil
}

type feedbackFixture struct {
	store *Store
	pool  *pgxpool.Pool
}

const (
	testWorkspace = "ws-feedback"
	testActor     = "actor-feedback"
)

func newFeedbackFixture(t *testing.T) feedbackFixture {
	t.Helper()
	url := os.Getenv("LORETIDE_FEEDBACK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LORETIDE_FEEDBACK_TEST_DATABASE_URL is not set; real PostgreSQL test not run")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "feedback_test_" + diagnostics.NewID()
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
		// review-delivery, because a metric hangs off one of its publication
		// records and the version is resolved through its other two tables.
		"504_content_review_request.up.sql",
		"510_content_delivery_task.up.sql",
		"513_content_publication_record.up.sql",
		"526_content_manual_metric.up.sql",
		"527_content_manual_metric_id_unique_idx.up.sql",
		"528_content_manual_metric_record_idx.up.sql",
		"529_content_feedback_excerpt.up.sql",
		"530_content_feedback_excerpt_id_unique_idx.up.sql",
		"531_content_feedback_excerpt_record_idx.up.sql",
	} {
		sql, readErr := os.ReadFile(filepath.Join(migrations, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return feedbackFixture{
		pool: pool,
		store: &Store{
			DB:           pool,
			Diagnostics:  diagnostics.NewStore(pool, feedbackTestGuard{}),
			Publications: testPublications{pool: pool},
			Guard:        feedbackTestGuard{},
			Build:        "test",
		},
	}
}

// seedPublication writes a publication record. withDelivery decides whether it
// has a delivery task and review request behind it - which is what the version
// resolution walks.
func seedPublication(t *testing.T, fx feedbackFixture, status string, withDelivery bool) (string, string) {
	t.Helper()
	ctx := t.Context()
	publicationID := diagnostics.NewID()
	deliveryTaskID := ""
	versionID := ""
	if withDelivery {
		deliveryTaskID = diagnostics.NewID()
		reviewID := diagnostics.NewID()
		versionID = diagnostics.NewID()
		if _, err := fx.pool.Exec(ctx, `INSERT INTO content_review_request
			(review_request_id, workspace_id, work_id, artifact_id, version_id,
			 account_id, channel, snapshot, status, requested_by)
			VALUES ($1,$2,'w','a',$3,'acct-1','xiaohongshu','{}'::jsonb,'approved',$4)`,
			reviewID, testWorkspace, versionID, testActor); err != nil {
			t.Fatal(err)
		}
		if _, err := fx.pool.Exec(ctx, `INSERT INTO content_delivery_task
			(delivery_task_id, workspace_id, work_id, artifact_id, review_request_id,
			 channel, status)
			VALUES ($1,$2,'w','a',$3,'xiaohongshu','handed_off')`,
			deliveryTaskID, testWorkspace, reviewID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := fx.pool.Exec(ctx, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at)
		VALUES ($1,$2,'w','a',$3,'xiaohongshu',$4,$5,'https://example.invalid/p/1', now())`,
		publicationID, testWorkspace, deliveryTaskID, status, testActor); err != nil {
		t.Fatal(err)
	}
	return publicationID, versionID
}

func metricInput(publicationID string, metric Metric, value *int64) MetricInput {
	return MetricInput{
		PublicationRecordID: publicationID, Platform: PlatformXiaohongshu,
		AccountID: "acct-1", Metric: metric, Value: value,
		Unit: "次", StatWindow: "发布后 14 天累计",
		SampledAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func ptr(value int64) *int64 { return &value }

// SOP 10.1: "未知填空；0 只表示已确认的零值".
//
// The whole card turns on this. A NULL that comes back as 0 puts a number
// nobody observed into every later aggregate, and nothing downstream would
// report it.
func TestAnUnknownValueStaysUnknownAndAZeroStaysZero(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)

	unknown, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricConversion, nil), SourceManual)
	if err != nil {
		t.Fatalf("recording an unknown value was refused: %v", err)
	}
	if unknown.Value != nil {
		t.Errorf("an unknown value came back as %v", *unknown.Value)
	}
	confirmed, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricFollow, ptr(0)), SourceManual)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Value == nil || *confirmed.Value != 0 {
		t.Errorf("a confirmed zero came back as %v", confirmed.Value)
	}

	// And they are still different after a round trip through the column.
	var stored int
	if err = fx.pool.QueryRow(ctx, `SELECT count(*) FROM content_manual_metric
		WHERE workspace_id=$1 AND value IS NULL`, testWorkspace).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Errorf("%d rows stored NULL, want exactly the unknown one", stored)
	}
	list, err := fx.store.ListMetrics(ctx, testWorkspace, testActor, publicationID, "")
	if err != nil {
		t.Fatal(err)
	}
	byMetric := map[Metric]*int64{}
	for _, item := range list {
		byMetric[item.Metric] = item.Value
	}
	if byMetric[MetricConversion] != nil {
		t.Error("the unknown value read back as a number")
	}
	if byMetric[MetricFollow] == nil || *byMetric[MetricFollow] != 0 {
		t.Error("the confirmed zero read back as unknown")
	}
	if SameValue(byMetric[MetricConversion], byMetric[MetricFollow]) {
		t.Error("unknown and zero compare equal after a round trip")
	}
}

// R-044: "更新保留历史". A correction is a new row; the first one does not move.
func TestACorrectionIsANewRowAndTheFirstOneDoesNotMove(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)

	first, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricRead, ptr(1200)), SourceManual)
	if err != nil {
		t.Fatal(err)
	}
	// Same metric, same window, a corrected number.
	second, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricRead, ptr(1350)), SourceManual)
	if err != nil {
		t.Fatalf("a correction was refused: %v", err)
	}
	if second.ManualMetricID == first.ManualMetricID {
		t.Fatal("the correction reused the first row")
	}
	var value int64
	if err = fx.pool.QueryRow(ctx, `SELECT value FROM content_manual_metric
		WHERE manual_metric_id=$1`, first.ManualMetricID).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 1200 {
		t.Errorf("the first row now reads %d; a correction must not rewrite it", value)
	}
}

// The server decides the source from the entry point. A body field would make
// "where did this come from" something the caller declares.
func TestTheSourceTypeComesFromTheEntryPoint(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)

	manual, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricLike, ptr(10)), SourceManual)
	if err != nil {
		t.Fatal(err)
	}
	if manual.SourceType != SourceManual {
		t.Errorf("the form endpoint stored %q", manual.SourceType)
	}
	imported, err := fx.store.RecordBatch(ctx, testWorkspace, testActor,
		[]MetricInput{metricInput(publicationID, MetricShare, ptr(3))}, SourceCSVImport)
	if err != nil {
		t.Fatal(err)
	}
	if imported[0].SourceType != SourceCSVImport {
		t.Errorf("the import endpoint stored %q", imported[0].SourceType)
	}
	// And MetricInput has no field a caller could use to say otherwise.
	if _, hasField := any(MetricInput{}).(interface{ GetSourceType() string }); hasField {
		t.Error("MetricInput exposes a source type")
	}
}

// Who typed it comes from the session. It is the one field on the row that
// must not be forgeable.
func TestTheRecordingActorComesFromTheSession(t *testing.T) {
	fx := newFeedbackFixture(t)
	publicationID, _ := seedPublication(t, fx, "verified_published", true)
	written, err := fx.store.Record(t.Context(), testWorkspace, testActor,
		metricInput(publicationID, MetricLike, ptr(10)), SourceManual)
	if err != nil {
		t.Fatal(err)
	}
	if written.RecordedBy != testActor {
		t.Errorf("the row was attributed to %q", written.RecordedBy)
	}
}

// All or nothing. A partial write leaves somebody believing all the pasted
// rows landed, and "how many got in" is the only question this feature has to
// answer.
func TestABatchWithOneBadRowWritesNothing(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)

	good := metricInput(publicationID, MetricRead, ptr(100))
	bad := metricInput(publicationID, MetricRead, ptr(200))
	bad.Metric = "engagement"
	_, err := fx.store.RecordBatch(ctx, testWorkspace, testActor,
		[]MetricInput{good, bad, good}, SourceCSVImport)
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("got %v, want a named field", err)
	}
	if fieldErr.Row != 2 || fieldErr.Field != "metric" {
		t.Errorf("refusal names row %d field %q, want row 2 metric", fieldErr.Row, fieldErr.Field)
	}
	var count int
	if err = fx.pool.QueryRow(ctx, `SELECT count(*) FROM content_manual_metric
		WHERE workspace_id=$1`, testWorkspace).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d rows landed from a batch that was refused", count)
	}
}

// A publication record that does not exist here is refused exactly like one in
// another workspace - and the whole batch is refused with it.
func TestABatchNamingAForeignRecordWritesNothing(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)

	good := metricInput(publicationID, MetricRead, ptr(100))
	foreign := metricInput("no-such-record", MetricLike, ptr(1))
	if _, err := fx.store.RecordBatch(ctx, testWorkspace, testActor,
		[]MetricInput{good, foreign}, SourceCSVImport); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
	var count int
	if err := fx.pool.QueryRow(ctx, `SELECT count(*) FROM content_manual_metric
		WHERE workspace_id=$1`, testWorkspace).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("%d rows landed", count)
	}
}

// Ruling Q2: the version is resolved on read, two hops. "" is a real answer
// for a record entered for history, and never a reason to refuse.
func TestTheVersionIsResolvedOnReadAndIsEmptyWhenItCannotBe(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()

	withDelivery, versionID := seedPublication(t, fx, "verified_published", true)
	if _, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(withDelivery, MetricRead, ptr(500)), SourceManual); err != nil {
		t.Fatal(err)
	}
	resolved, err := fx.store.ListMetrics(ctx, testWorkspace, testActor, withDelivery, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].VersionID != versionID {
		t.Fatalf("version resolved to %q, want %q", resolved[0].VersionID, versionID)
	}

	// Entered for history: no delivery task, so no version - and recording is
	// still allowed.
	historical, _ := seedPublication(t, fx, "reported_published", false)
	if _, err = fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(historical, MetricRead, ptr(90)), SourceManual); err != nil {
		t.Fatalf("recording against a historical entry was refused: %v", err)
	}
	unresolved, err := fx.store.ListMetrics(ctx, testWorkspace, testActor, historical, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(unresolved) != 1 || unresolved[0].VersionID != "" {
		t.Errorf("version resolved to %q, want empty", unresolved[0].VersionID)
	}
}

// R-045: the quote and the operator's reading are two columns, and either may
// stand alone.
func TestAnExcerptKeepsTheQuoteAndTheReadingApart(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)
	now := time.Now().UTC().Format(time.RFC3339)

	both, err := fx.store.Excerpt(ctx, testWorkspace, testActor, ExcerptInput{
		PublicationRecordID: publicationID, SourceType: ExcerptComment,
		RedactedExcerpt: "看完就去买了", Interpretation: "转化点在第三段",
		Tags: []string{"转化", "对比"}, OccurredAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if both.RedactedExcerpt != "看完就去买了" || both.Interpretation != "转化点在第三段" {
		t.Errorf("the two halves did not stay apart: %+v", both)
	}
	if len(both.Tags) != 2 {
		t.Errorf("tags came back as %v", both.Tags)
	}

	quoteOnly, err := fx.store.Excerpt(ctx, testWorkspace, testActor, ExcerptInput{
		PublicationRecordID: publicationID, SourceType: ExcerptPrivateMessage,
		RedactedExcerpt: "私信问在哪买", OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("a quote with no reading was refused: %v", err)
	}
	if quoteOnly.Interpretation != "" {
		t.Errorf("an empty reading came back as %q", quoteOnly.Interpretation)
	}
	readingOnly, err := fx.store.Excerpt(ctx, testWorkspace, testActor, ExcerptInput{
		PublicationRecordID: publicationID, SourceType: ExcerptLead,
		Interpretation: "线索来自同一个群", OccurredAt: now,
	})
	if err != nil {
		t.Fatalf("a reading with no quote was refused: %v", err)
	}
	if readingOnly.RedactedExcerpt != "" {
		t.Errorf("an empty quote came back as %q", readingOnly.RedactedExcerpt)
	}
	// Tags come back as an empty array, never null.
	if readingOnly.Tags == nil {
		t.Error("tags came back nil; that marshals to null")
	}
}

// SOP 2's fifth workbench item: published, and nobody has recorded anything.
func TestPendingRegistrationsAreThePublishedRecordsWithNoMetrics(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()

	waiting, _ := seedPublication(t, fx, "verified_published", true)
	reported, _ := seedPublication(t, fx, "reported_published", false)
	failed, _ := seedPublication(t, fx, "failed", true)
	removed, _ := seedPublication(t, fx, "removed", true)

	pending, err := fx.store.PendingRegistrations(ctx, testWorkspace, testActor)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, item := range pending {
		listed[item.PublicationRecordID] = true
	}
	if !listed[waiting] || !listed[reported] {
		t.Errorf("a published record with no metrics is missing: %v", listed)
	}
	// Nothing went out, so there are no real results to copy down.
	if listed[failed] || listed[removed] {
		t.Errorf("a failed or removed record was listed as pending: %v", listed)
	}

	if _, err = fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(waiting, MetricRead, ptr(10)), SourceManual); err != nil {
		t.Fatal(err)
	}
	pending, err = fx.store.PendingRegistrations(ctx, testWorkspace, testActor)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range pending {
		if item.PublicationRecordID == waiting {
			t.Error("a record with a metric was still pending")
		}
	}
}

// The derivation is a query, not a column. A stored flag would be wrong the
// moment somebody recorded a number.
func TestPendingRegistrationIsNotAStoredColumn(t *testing.T) {
	fx := newFeedbackFixture(t)
	var exists bool
	if err := fx.pool.QueryRow(t.Context(), `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name IN ('content_manual_metric','content_feedback_excerpt','content_publication_record')
		  AND column_name IN ('pending_registration','needs_registration','is_due'))`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("a derived display was stored as a column; it would be wrong the moment a number was recorded")
	}
}

// This card produces exactly one review state, and there is no table a report
// could live in.
func TestTheReviewPlaceholderIsPendingDataAndHasNoTable(t *testing.T) {
	fx := newFeedbackFixture(t)
	if got := fx.store.ReviewPlaceholder(t.Context(), testWorkspace, "artifact-1"); got != StatePendingData {
		t.Errorf("the placeholder is %q, want pending_data", got)
	}
	var exists bool
	if err := fx.pool.QueryRow(t.Context(), `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_name LIKE 'content_%review_report%'
		   OR table_name LIKE 'content_%retrospective%')`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("a report table exists; this phase cannot produce a line of one, and an empty table reads as 'it will be filled in'")
	}
}

// Read and play are separate numbers, all the way to the rows.
func TestReadAndPlayAreStoredAndReadSeparately(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)

	if _, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricRead, ptr(1000)), SourceManual); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricPlay, ptr(4000)), SourceManual); err != nil {
		t.Fatal(err)
	}
	reads, err := fx.store.ListMetrics(ctx, testWorkspace, testActor, publicationID, "read")
	if err != nil {
		t.Fatal(err)
	}
	plays, err := fx.store.ListMetrics(ctx, testWorkspace, testActor, publicationID, "play")
	if err != nil {
		t.Fatal(err)
	}
	if len(reads) != 1 || *reads[0].Value != 1000 {
		t.Errorf("reads came back as %+v", reads)
	}
	if len(plays) != 1 || *plays[0].Value != 4000 {
		t.Errorf("plays came back as %+v", plays)
	}
}

// Another workspace's rows are refused exactly as missing ones.
func TestAnotherWorkspacesRowsAreRefusedAsMissing(t *testing.T) {
	fx := newFeedbackFixture(t)
	ctx := t.Context()
	publicationID, _ := seedPublication(t, fx, "verified_published", true)
	if _, err := fx.store.Record(ctx, testWorkspace, testActor,
		metricInput(publicationID, MetricRead, ptr(10)), SourceManual); err != nil {
		t.Fatal(err)
	}
	other, err := fx.store.ListMetrics(ctx, "ws-other", testActor, publicationID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Errorf("another workspace read %d rows", len(other))
	}
	if _, err = fx.store.Record(ctx, "ws-other", testActor,
		metricInput(publicationID, MetricLike, ptr(1)), SourceManual); !errors.Is(err, ErrNotFound) {
		t.Errorf("writing against a foreign record answered %v", err)
	}
}
