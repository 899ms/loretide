package reviewdelivery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// Review requests, delivery tasks and publication records against real
// PostgreSQL.
//
// Contract: specs/025-review-delivery-manual/contracts/review-delivery.md
//
// Runs only with LORETIDE_REVIEW_TEST_DATABASE_URL set; newReviewFixture skips
// otherwise, and a skip is not a pass.

// reviewTestGuard stands in for workspace-core's fence. This fixture runs in an
// isolated schema holding the content tables only, so there is no workspace row
// to lock here. The fence itself is proven against the real schema by
// internal/handler's TestReviewDeliveryWritesAreFencedByWorkspaceDeletion.
type reviewTestGuard struct{}

func (reviewTestGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	return nil
}

// testArtifacts answers the two questions the adapter answers in production:
// does this version belong here, and is the document a channel draft. It reads
// the real work-editor tables, which the fixture migrates alongside this
// module's own.
type testArtifacts struct{ pool *pgxpool.Pool }

func (a testArtifacts) ResolveVersion(ctx context.Context, workspaceID, artifactID, versionID string) (string, string, error) {
	var workID, kind string
	var err error
	if versionID == "" {
		err = a.pool.QueryRow(ctx, `SELECT work_id, kind FROM content_artifact
			WHERE workspace_id=$1 AND artifact_id=$2`, workspaceID, artifactID).Scan(&workID, &kind)
	} else {
		err = a.pool.QueryRow(ctx, `SELECT a.work_id, a.kind FROM content_artifact a
			JOIN content_artifact_version v
			  ON v.workspace_id=a.workspace_id AND v.artifact_id=a.artifact_id
			WHERE a.workspace_id=$1 AND a.artifact_id=$2 AND v.version_id=$3`,
			workspaceID, artifactID, versionID).Scan(&workID, &kind)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", ErrStorage
	}
	return workID, kind, nil
}

type reviewFixture struct {
	store *Store
	pool  *pgxpool.Pool
	now   *time.Time
}

const (
	testWorkspace = "ws-review"
	testActor     = "actor-review"
	testAccount   = "acct-1"
)

func newReviewFixture(t *testing.T) reviewFixture {
	t.Helper()
	url := os.Getenv("LORETIDE_REVIEW_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LORETIDE_REVIEW_TEST_DATABASE_URL is not set; real PostgreSQL test not run")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "review_test_" + diagnostics.NewID()
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
		// work-editor, because a review binds one of its versions.
		"494_content_work.up.sql",
		"497_content_artifact.up.sql",
		"500_content_artifact_version.up.sql",
		"504_content_review_request.up.sql",
		"505_content_review_request_id_unique_idx.up.sql",
		"506_content_review_request_workspace_idx.up.sql",
		"507_content_review_transition.up.sql",
		"508_content_review_transition_id_unique_idx.up.sql",
		"509_content_review_transition_subject_idx.up.sql",
		"510_content_delivery_task.up.sql",
		"511_content_delivery_task_id_unique_idx.up.sql",
		"512_content_delivery_task_workspace_idx.up.sql",
		"513_content_publication_record.up.sql",
		"514_content_publication_record_id_unique_idx.up.sql",
		"515_content_publication_record_artifact_idx.up.sql",
	} {
		sql, readErr := os.ReadFile(filepath.Join(migrations, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	clock := time.Now().UTC()
	fixture := reviewFixture{pool: pool, now: &clock}
	fixture.store = &Store{
		DB:          pool,
		Diagnostics: diagnostics.NewStore(pool, reviewTestGuard{}),
		Artifacts:   testArtifacts{pool: pool},
		Guard:       reviewTestGuard{},
		Build:       "test",
		Now:         func() time.Time { return *fixture.now },
	}
	return fixture
}

// seedDraft creates a work, one channel draft and one saved version of it,
// returning the document id and the version id.
func seedDraft(t *testing.T, fx reviewFixture, kind string) (string, string) {
	t.Helper()
	ctx := t.Context()
	workID := diagnostics.NewID()
	artifactID := diagnostics.NewID()
	versionID := diagnostics.NewID()
	if _, err := fx.pool.Exec(ctx, `INSERT INTO content_work
		(work_id, workspace_id, topic_card_id, snapshot_id, title)
		VALUES ($1,$2,'card-1','','第一篇')`, workID, testWorkspace); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.pool.Exec(ctx, `INSERT INTO content_artifact
		(artifact_id, work_id, workspace_id, kind, title, position)
		VALUES ($1,$2,$3,$4,'小红书稿',1)`, artifactID, workID, testWorkspace, kind); err != nil {
		t.Fatal(err)
	}
	saveVersion(t, fx, workID, artifactID, versionID, 1, "第三版正文")
	return artifactID, versionID
}

func saveVersion(t *testing.T, fx reviewFixture, workID, artifactID, versionID string, revision int, body string) {
	t.Helper()
	if _, err := fx.pool.Exec(t.Context(), `INSERT INTO content_artifact_version
		(version_id, artifact_id, work_id, workspace_id, revision, source, action, body, actor_id)
		VALUES ($1,$2,$3,$4,$5,'edited','saved',$6,$7)`,
		versionID, artifactID, workID, testWorkspace, revision, body, testActor); err != nil {
		t.Fatal(err)
	}
}

func submitDraft(t *testing.T, fx reviewFixture) ReviewRequest {
	t.Helper()
	artifactID, versionID := seedDraft(t, fx, "channel_draft")
	request, err := fx.store.Submit(t.Context(), testWorkspace, testActor, SubmitRequest{
		ArtifactID: artifactID, VersionID: versionID,
		AccountID: testAccount, Channel: ChannelXiaohongshu,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return request
}

func approve(t *testing.T, fx reviewFixture, reviewID string) {
	t.Helper()
	if _, err := fx.store.Decide(t.Context(), testWorkspace, testActor, reviewID,
		ReviewApproved, "看过了，可以发"); err != nil {
		t.Fatalf("approve: %v", err)
	}
}

// SC-001. The snapshot is frozen at submit time; three more versions and a
// disposition must not move a byte of it.
func TestTheFrozenSnapshotSurvivesNewVersionsAndADecision(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	request := submitDraft(t, fx)

	var before []byte
	if err := fx.pool.QueryRow(ctx, `SELECT snapshot FROM content_review_request
		WHERE review_request_id=$1`, request.ReviewRequestID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for revision := 2; revision <= 4; revision++ {
		saveVersion(t, fx, request.WorkID, request.ArtifactID, diagnostics.NewID(), revision, "后来又改了")
	}
	if _, err := fx.store.Decide(ctx, testWorkspace, testActor, request.ReviewRequestID,
		ReviewChangesRequested, "开头再紧一点"); err != nil {
		t.Fatal(err)
	}
	var after []byte
	if err := fx.pool.QueryRow(ctx, `SELECT snapshot FROM content_review_request
		WHERE review_request_id=$1`, request.ReviewRequestID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("the frozen snapshot changed:\nbefore %s\nafter  %s", before, after)
	}

	// And the version it bound still reads back byte for byte.
	var body string
	if err := fx.pool.QueryRow(ctx, `SELECT body FROM content_artifact_version
		WHERE version_id=$1`, request.VersionID).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if body != "第三版正文" {
		t.Errorf("the reviewed version now reads %q", body)
	}
}

// SC-002. Re-submitting is ordinary, not a conflict.
func TestTheSameDocumentCanBeSubmittedTwiceAndTheFirstIsUntouched(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	first := submitDraft(t, fx)
	second, err := fx.store.Submit(ctx, testWorkspace, testActor, SubmitRequest{
		ArtifactID: first.ArtifactID, VersionID: first.VersionID,
		AccountID: testAccount, Channel: ChannelXiaohongshu,
	})
	if err != nil {
		t.Fatalf("a second submission was refused: %v", err)
	}
	if second.ReviewRequestID == first.ReviewRequestID {
		t.Fatal("the second submission reused the first request")
	}
	reread, err := fx.store.GetReview(ctx, testWorkspace, testActor, first.ReviewRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Status != ReviewPending || reread.RequestedAt != first.RequestedAt {
		t.Errorf("the first request moved: %+v", reread)
	}
}

// SC-015. SOP 8's object of review is a specific channel's version.
func TestOnlyAChannelDraftCanBeSubmitted(t *testing.T) {
	fx := newReviewFixture(t)
	artifactID, versionID := seedDraft(t, fx, "body")
	_, err := fx.store.Submit(t.Context(), testWorkspace, testActor, SubmitRequest{
		ArtifactID: artifactID, VersionID: versionID,
		AccountID: testAccount, Channel: ChannelXiaohongshu,
	})
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "kind" {
		t.Fatalf("got %v, want a refusal naming kind", err)
	}
}

// SC-017. SOP 8: "系统不得把'已通过'自动套在新版本上".
func TestAnApprovalDoesNotCarryToANewVersion(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	request := submitDraft(t, fx)
	approve(t, fx, request.ReviewRequestID)

	status, found, err := fx.store.LatestReviewStatusFor(ctx, testWorkspace, testActor,
		request.ArtifactID, request.VersionID)
	if err != nil || !found || status != ReviewApproved {
		t.Fatalf("the approved version reads back as %q (found %v, err %v)", status, found, err)
	}
	newVersion := diagnostics.NewID()
	saveVersion(t, fx, request.WorkID, request.ArtifactID, newVersion, 2, "又写了一版")
	_, found, err = fx.store.LatestReviewStatusFor(ctx, testWorkspace, testActor,
		request.ArtifactID, newVersion)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("the new version inherited a review; approval belongs to the combination it was granted for")
	}
}

// Every status change leaves exactly one row, including the creation.
func TestEveryStatusChangeWritesOneTransitionRow(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	request := submitDraft(t, fx)
	approve(t, fx, request.ReviewRequestID)

	transitions, err := fx.store.ListTransitions(ctx, testWorkspace, testActor,
		SubjectReviewRequest, request.ReviewRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 2 {
		t.Fatalf("got %d transition rows, want 2 (created, approved)", len(transitions))
	}
	if transitions[0].FromStatus != "" || transitions[0].ToStatus != "pending" {
		t.Errorf("the creation row reads %q -> %q", transitions[0].FromStatus, transitions[0].ToStatus)
	}
	if transitions[1].FromStatus != "pending" || transitions[1].ToStatus != "approved" {
		t.Errorf("the decision row reads %q -> %q", transitions[1].FromStatus, transitions[1].ToStatus)
	}
	if transitions[1].Reason != "看过了，可以发" {
		t.Errorf("the decision note did not reach the transition row: %q", transitions[1].Reason)
	}
	if transitions[1].ActorID != testActor {
		t.Errorf("the transition row does not say who did it: %q", transitions[1].ActorID)
	}
}

// SC-019. Three cases, because "refused" and "refused forever" are different
// answers and only the sequence tells them apart.
func TestATaskCannotPassReadyUntilItsReviewIsApproved(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	request := submitDraft(t, fx)

	task, err := fx.store.CreateTask(ctx, testWorkspace, testActor, CreateTaskRequest{
		ArtifactID: request.ArtifactID, Channel: ChannelXiaohongshu,
		ReviewRequestID: request.ReviewRequestID,
	})
	if err != nil {
		t.Fatalf("drafting a task against a pending review was refused: %v", err)
	}
	_, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
		DeliveryAdvance{To: DeliveryReady})
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "review_request_id" {
		t.Fatalf("ready with a pending review gave %v, want a refusal naming review_request_id", err)
	}
	approve(t, fx, request.ReviewRequestID)
	if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
		DeliveryAdvance{To: DeliveryReady}); err != nil {
		t.Fatalf("ready after approval was still refused: %v", err)
	}
}

// SC-012. SOP 9.1: "导出成功、复制完成或交接给他人都不自动等于发布成功".
// Three separate cases: one loop over the methods is the whole point.
func TestHandingOverByAnyMethodProducesNoPublicationRecord(t *testing.T) {
	for _, method := range HandoffMethods {
		t.Run(string(method), func(t *testing.T) {
			fx := newReviewFixture(t)
			ctx := t.Context()
			request := submitDraft(t, fx)
			approve(t, fx, request.ReviewRequestID)
			task, err := fx.store.CreateTask(ctx, testWorkspace, testActor, CreateTaskRequest{
				ArtifactID: request.ArtifactID, Channel: ChannelXiaohongshu,
				ReviewRequestID: request.ReviewRequestID,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
				DeliveryAdvance{To: DeliveryReady}); err != nil {
				t.Fatal(err)
			}
			if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
				DeliveryAdvance{To: DeliveryHandedOff, HandoffMethod: method}); err != nil {
				t.Fatal(err)
			}

			records, err := fx.store.ListPublications(ctx, testWorkspace, testActor, request.ArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 0 {
				t.Errorf("handing over by %s produced %d publication records", method, len(records))
			}
			_, published, err := fx.store.CurrentPublication(ctx, testWorkspace, testActor, request.ArtifactID)
			if err != nil {
				t.Fatal(err)
			}
			if published {
				t.Errorf("handing over by %s made the piece read as published", method)
			}
		})
	}
}

// SC-013. The workbench keeps saying 待登记 until a person writes something
// down, and it says it because nobody wrote anything - not because the system
// asked a platform.
func TestPendingRegistrationAppearsAfterHandoverAndGoesAwayOnceRecorded(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	task := handOver(t, fx)

	reread, err := fx.store.GetTask(ctx, testWorkspace, testActor, task.DeliveryTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if !reread.PendingRegistration {
		t.Fatal("a handed-over task with no record was not pending registration")
	}
	if _, err = fx.store.Record(ctx, testWorkspace, testActor, RecordRequest{
		ArtifactID: task.ArtifactID, DeliveryTaskID: task.DeliveryTaskID,
		Channel: ChannelXiaohongshu, Status: PublicationReported,
		PageURLOrContentID: "https://example.invalid/p/1",
	}); err != nil {
		t.Fatal(err)
	}
	reread, err = fx.store.GetTask(ctx, testWorkspace, testActor, task.DeliveryTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.PendingRegistration {
		t.Error("the task was still pending registration after a record was written")
	}
}

// And the other half: the storage layer has no such column to be wrong.
func TestPendingRegistrationIsNotAStoredColumn(t *testing.T) {
	fx := newReviewFixture(t)
	var exists bool
	if err := fx.pool.QueryRow(t.Context(), `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name IN ('content_delivery_task','content_publication_record')
		  AND column_name IN ('pending_registration','due','current_status'))`).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("a derived display was stored as a column; it would be wrong a minute after it was written")
	}
}

// SC-013a. Due is a comparison against the clock, made when the list is read.
func TestDueAppearsOnlyOnceThePlannedTimeHasPassed(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	request := submitDraft(t, fx)
	approve(t, fx, request.ReviewRequestID)
	task, err := fx.store.CreateTask(ctx, testWorkspace, testActor, CreateTaskRequest{
		ArtifactID: request.ArtifactID, Channel: ChannelXiaohongshu,
		ReviewRequestID: request.ReviewRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
		DeliveryAdvance{To: DeliveryReady}); err != nil {
		t.Fatal(err)
	}
	planned := (*fx.now).Add(time.Hour)
	if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
		DeliveryAdvance{To: DeliveryScheduled, ScheduledAt: &planned}); err != nil {
		t.Fatal(err)
	}
	reread, err := fx.store.GetTask(ctx, testWorkspace, testActor, task.DeliveryTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.Due {
		t.Error("a task planned for an hour from now was already due")
	}

	// Move the clock, not the row. Nothing woke up in between.
	*fx.now = planned.Add(time.Minute)
	reread, err = fx.store.GetTask(ctx, testWorkspace, testActor, task.DeliveryTaskID)
	if err != nil {
		t.Fatal(err)
	}
	if !reread.Due {
		t.Error("the planned time passed and the task was not due")
	}
	if reread.Status != DeliveryScheduled {
		t.Errorf("the task's status changed by itself to %q; nothing may act on the planned time", reread.Status)
	}
}

// SC-014. SOP 9.3, both exits.
func TestAChangedTargetHoldsTheTaskAndKeepingTheOldSnapshotReleasesIt(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	request := submitDraft(t, fx)
	approve(t, fx, request.ReviewRequestID)
	task, err := fx.store.CreateTask(ctx, testWorkspace, testActor, CreateTaskRequest{
		ArtifactID: request.ArtifactID, Channel: ChannelXiaohongshu,
		ReviewRequestID: request.ReviewRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
		DeliveryAdvance{To: DeliveryReady}); err != nil {
		t.Fatal(err)
	}

	// Nothing changed yet: this must be a no-op, not a transition row saying
	// something happened.
	unchanged, held, err := fx.store.HoldForChangedTarget(ctx, testWorkspace, testActor,
		task.DeliveryTaskID, TargetOf(request.Snapshot))
	if err != nil {
		t.Fatal(err)
	}
	if held || unchanged.Status != DeliveryReady {
		t.Fatalf("an unchanged target held the task (held=%v, status=%q)", held, unchanged.Status)
	}

	newVersion := diagnostics.NewID()
	saveVersion(t, fx, request.WorkID, request.ArtifactID, newVersion, 2, "第五版")
	target := TargetOf(request.Snapshot)
	target.VersionID = newVersion
	after, held, err := fx.store.HoldForChangedTarget(ctx, testWorkspace, testActor, task.DeliveryTaskID, target)
	if err != nil {
		t.Fatal(err)
	}
	if !held || after.Status != DeliveryHeld {
		t.Fatalf("choosing a new version did not hold the task (held=%v, status=%q)", held, after.Status)
	}

	released, err := fx.store.KeepDeliveringApprovedSnapshot(ctx, testWorkspace, testActor,
		task.DeliveryTaskID, "封面已经排好了")
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != DeliveryReady {
		t.Fatalf("keeping the approved snapshot left the task at %q", released.Status)
	}
	transitions, err := fx.store.ListTransitions(ctx, testWorkspace, testActor,
		SubjectDeliveryTask, task.DeliveryTaskID)
	if err != nil {
		t.Fatal(err)
	}
	last := transitions[len(transitions)-1]
	if last.Reason == "" || last.ToStatus != "ready" {
		t.Errorf("the deliberate choice was not recorded: %+v", last)
	}
	heldRow := transitions[len(transitions)-2]
	if heldRow.ToStatus != "held" || heldRow.Reason == "" {
		t.Errorf("the automatic hold did not say what changed: %+v", heldRow)
	}
}

// Every one of the three target changes SOP 9.3 names, against real rows.
func TestChangingTheAccountOrTheChannelAlsoHoldsTheTask(t *testing.T) {
	for name, mutate := range map[string]func(DeliveryTarget) DeliveryTarget{
		"account":     func(target DeliveryTarget) DeliveryTarget { target.AccountID = "acct-2"; return target },
		"channel":     func(target DeliveryTarget) DeliveryTarget { target.Channel = ChannelDouyin; return target },
		"attachments": func(target DeliveryTarget) DeliveryTarget { target.Attachments = []string{"file-1"}; return target },
	} {
		t.Run(name, func(t *testing.T) {
			fx := newReviewFixture(t)
			ctx := t.Context()
			request := submitDraft(t, fx)
			approve(t, fx, request.ReviewRequestID)
			task, err := fx.store.CreateTask(ctx, testWorkspace, testActor, CreateTaskRequest{
				ArtifactID: request.ArtifactID, Channel: ChannelXiaohongshu,
				ReviewRequestID: request.ReviewRequestID,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
				DeliveryAdvance{To: DeliveryReady}); err != nil {
				t.Fatal(err)
			}
			after, held, err := fx.store.HoldForChangedTarget(ctx, testWorkspace, testActor,
				task.DeliveryTaskID, mutate(TargetOf(request.Snapshot)))
			if err != nil {
				t.Fatal(err)
			}
			if !held || after.Status != DeliveryHeld {
				t.Errorf("changing the %s did not hold the task", name)
			}
		})
	}
}

// FR-019. The current status is the latest row, and the older rows do not move.
func TestTheCurrentStatusIsTheLatestRowAndOlderRowsAreUntouched(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	task := handOver(t, fx)

	first, err := fx.store.Record(ctx, testWorkspace, testActor, RecordRequest{
		ArtifactID: task.ArtifactID, DeliveryTaskID: task.DeliveryTaskID,
		Channel: ChannelXiaohongshu, Status: PublicationReported,
		PageURLOrContentID: "https://example.invalid/p/1",
		DeclaredBy:         "运营同事小王",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.Record(ctx, testWorkspace, testActor, RecordRequest{
		ArtifactID: task.ArtifactID, Channel: ChannelXiaohongshu,
		Status: PublicationVerified, PageURLOrContentID: "https://example.invalid/p/1",
		VerificationNote: "我自己点开看过", VersionMatch: VersionMatched,
	}); err != nil {
		t.Fatal(err)
	}
	// A platform taking the piece down after it was verified is the ordinary
	// case. Refusing it would stop someone recording what happened.
	if _, err = fx.store.Record(ctx, testWorkspace, testActor, RecordRequest{
		ArtifactID: task.ArtifactID, Channel: ChannelXiaohongshu,
		Status: PublicationRemoved, ReceiptNote: "链接失效了，平台删了",
	}); err != nil {
		t.Fatalf("a removal after a verification was refused: %v", err)
	}

	current, found, err := fx.store.CurrentPublication(ctx, testWorkspace, testActor, task.ArtifactID)
	if err != nil || !found {
		t.Fatalf("no current publication (found %v, err %v)", found, err)
	}
	if current.Status != PublicationRemoved {
		t.Errorf("the current status is %q, want the latest row's", current.Status)
	}
	reread, err := fx.store.ListPublications(ctx, testWorkspace, testActor, task.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reread) != 3 {
		t.Fatalf("got %d records, want 3", len(reread))
	}
	oldest := reread[len(reread)-1]
	if oldest.PublicationRecordID != first.PublicationRecordID ||
		oldest.DeclaredBy != "运营同事小王" || oldest.Status != PublicationReported {
		t.Errorf("the first record moved: %+v", oldest)
	}
}

// SC-021. The declarer is what the session says, and the request cannot carry
// one at all.
func TestTheRecordingActorCannotComeFromTheRequest(t *testing.T) {
	fields := reflect.TypeOf(RecordRequest{})
	for i := range fields.NumField() {
		name := strings.ToLower(fields.Field(i).Name)
		if name == "actorid" || name == "actor" {
			t.Fatalf("RecordRequest carries %s; the one unforgeable field on the row would come from the body",
				fields.Field(i).Name)
		}
	}
	fx := newReviewFixture(t)
	task := handOver(t, fx)
	record, err := fx.store.Record(t.Context(), testWorkspace, testActor, RecordRequest{
		ArtifactID: task.ArtifactID, Channel: ChannelXiaohongshu,
		Status: PublicationReported, PageURLOrContentID: "content-id-9",
		DeclaredBy: "someone else entirely",
	})
	if err != nil {
		t.Fatal(err)
	}
	if record.ActorID != testActor {
		t.Errorf("the record was attributed to %q", record.ActorID)
	}
}

// A request, task or record from another workspace is refused exactly as a
// missing one.
func TestAnotherWorkspacesRowsAreRefusedAsMissing(t *testing.T) {
	fx := newReviewFixture(t)
	ctx := t.Context()
	request := submitDraft(t, fx)
	if _, err := fx.store.GetReview(ctx, "ws-other", testActor, request.ReviewRequestID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a foreign review request answered %v", err)
	}
	task := handOver(t, fx)
	if _, err := fx.store.GetTask(ctx, "ws-other", testActor, task.DeliveryTaskID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a foreign delivery task answered %v", err)
	}
}

// handOver drives one piece all the way to handed_off and returns its task.
func handOver(t *testing.T, fx reviewFixture) DeliveryTask {
	t.Helper()
	ctx := t.Context()
	request := submitDraft(t, fx)
	approve(t, fx, request.ReviewRequestID)
	task, err := fx.store.CreateTask(ctx, testWorkspace, testActor, CreateTaskRequest{
		ArtifactID: request.ArtifactID, Channel: ChannelXiaohongshu,
		ReviewRequestID: request.ReviewRequestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
		DeliveryAdvance{To: DeliveryReady}); err != nil {
		t.Fatal(err)
	}
	handed, err := fx.store.Advance(ctx, testWorkspace, testActor, task.DeliveryTaskID,
		DeliveryAdvance{To: DeliveryHandedOff, HandoffMethod: HandoffExport})
	if err != nil {
		t.Fatal(err)
	}
	return handed
}
