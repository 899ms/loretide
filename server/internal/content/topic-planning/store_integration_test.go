package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// topicTestGuard stands in for workspace-core's fence. This fixture runs in an
// isolated schema that holds the content tables only, so there is no workspace
// row to lock here. The fence itself is proven against the real schema by
// internal/handler's TestContentTopicWritesAreFencedByWorkspaceDeletion.
type topicTestGuard struct{}

func (topicTestGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	return nil
}

type topicFixture struct {
	store *Store
	db    *testutil.Fixture
}

type testAccountReader struct{ pool *pgxpool.Pool }

func (r testAccountReader) Get(ctx context.Context, workspaceID, accountID string) (ipprofile.Account, error) {
	var account ipprofile.Account
	var settings []byte
	// Platform is read as the product reads it (GetContentAccount selects
	// it): a search theme on an account must be on that account's platform
	// (specs/036 FR-011), and a reader that left it "" would refuse them all.
	err := r.pool.QueryRow(ctx, `SELECT account_id, workspace_id, platform, settings FROM content_account
		WHERE workspace_id=$1 AND account_id=$2`, workspaceID, accountID).
		Scan(&account.AccountID, &account.WorkspaceID, &account.Platform, &settings)
	if errors.Is(err, pgx.ErrNoRows) {
		return ipprofile.Account{}, ipprofile.ErrNotFound
	}
	if err != nil {
		return ipprofile.Account{}, err
	}
	// Settings carry the stored material scope preference, which a start
	// records as saved_preference. Leaving them unread here would make the
	// preference silently default and the two scope fields agree by accident.
	if len(settings) > 0 {
		if err = json.Unmarshal(settings, &account.Settings); err != nil {
			return ipprofile.Account{}, err
		}
	}
	return account, nil
}

// The account's settings and its current persona revision, read from the same
// fixture schema. The revision is what a start pins and what readiness is
// judged from; it is served here rather than mocked so an integration test
// exercises the real profile JSON round trip.
func (r testAccountReader) CurrentPersonaRevision(ctx context.Context, workspaceID, accountID string) (ipprofile.Revision, error) {
	var revision ipprofile.Revision
	var profile []byte
	err := r.pool.QueryRow(ctx, `SELECT revision_id, account_id, workspace_id, revision, persona_prompt, profile
		FROM content_account_revision
		WHERE workspace_id=$1 AND account_id=$2
		ORDER BY revision DESC LIMIT 1`, workspaceID, accountID).
		Scan(&revision.RevisionID, &revision.AccountID, &revision.WorkspaceID,
			&revision.Revision, &revision.PersonaPrompt, &profile)
	if errors.Is(err, pgx.ErrNoRows) {
		return ipprofile.Revision{}, ipprofile.ErrNotFound
	}
	if err != nil {
		return ipprofile.Revision{}, err
	}
	if err = json.Unmarshal(profile, &revision.Profile); err != nil {
		return ipprofile.Revision{}, err
	}
	return revision, nil
}

type testSourceReader struct{ pool *pgxpool.Pool }

func (r testSourceReader) Exists(ctx context.Context, workspaceID, sourceID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM content_source WHERE workspace_id=$1 AND source_id=$2
	)`, workspaceID, sourceID).Scan(&exists)
	return exists, err
}

func newTopicFixture(t *testing.T) topicFixture {
	t.Helper()
	url := os.Getenv("LORETIDE_TOPIC_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("LORETIDE_TOPIC_TEST_DATABASE_URL is not set; real PostgreSQL test not run")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := "topic_test_" + diagnostics.NewID()
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
		"477_content_account.up.sql",
		// The account's revision history and its profile column: a start pins
		// the revision id and judges readiness from the profile on it.
		"479_content_account_revision.up.sql",
		"480_content_account_revision_unique_idx.up.sql",
		"482_content_account_revision_profile.up.sql",
		"483_content_topic_card.up.sql",
		"485_content_brief_revision.up.sql",
		"486_content_brief_revision_unique_idx.up.sql",
		"488_content_topic_card_id_unique_idx.up.sql",
		"489_content_brief_revision_id_unique_idx.up.sql",
		"490_content_start_snapshot.up.sql",
		"491_content_start_snapshot_id_unique_idx.up.sql",
		"492_content_start_snapshot_workspace_idx.up.sql",
		"493_content_start_snapshot_card_idx.up.sql",
		"516_content_source.up.sql",
		"517_content_source_id_unique_idx.up.sql",
		"536_content_topic_card_fit_source_ids.up.sql",
		"537_content_topic_card_evidence_source_ids.up.sql",
		// specs/035 PR 3: CreateOnce's origin key and its unique index.
		"611_content_topic_card_origin_key.up.sql",
		"612_content_topic_card_origin_key_idx.up.sql",
	} {
		sql, readErr := os.ReadFile(filepath.Join(migrations, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if _, err = pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	diagnosticStore := diagnostics.NewStore(pool, topicTestGuard{})
	return topicFixture{
		store: &Store{DB: pool, Diagnostics: diagnosticStore, Accounts: testAccountReader{pool},
			Sources: testSourceReader{pool},
			Guard: topicTestGuard{}, Build: "test"},
		db: testutil.New(pool, "", ""),
	}
}

func completeCard(workspace string) TopicCard {
	return TopicCard{
		WorkspaceID:               workspace,
		AudienceProblemJudgment:   "给刚起步的运营者，解决选题失焦，判断是先窄后宽",
		IPFit:                     "适合这个 IP 的实操定位",
		Timing:                    "没有时效依据",
		ExistingContentRelation:   "没有既有作品",
		EvidenceGapsAndInvestment: "没有现成证据，预计投入两小时",
		Channels:                  []string{"xiaohongshu", "wechat_mp"},
		RecommendedAction:         "开始",
	}
}

func completeBrief() BriefRevision {
	return BriefRevision{
		Audience:             "刚起步的运营者",
		CoreProblem:          "选题失焦",
		ClaimAndBoundaries:   "先窄后宽；不承诺流量结果",
		Channels:             []string{"xiaohongshu", "wechat_mp"},
		Format:               "图文",
		Structure:            "问题-证据-行动",
		CitationRequirements: "引用公开统计",
		SourceScope:          "用户粘贴的文本与 URL 字符串",
		Deliverable:          "一篇图文草稿",
		TimeLimit:            "两小时",
		CostLimit:            "零额外采购",
	}
}

func TestTopicCardsActionsAndBriefsRoundTrip(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-a", "actor-a"

	wantCard := completeCard(workspace)
	created, err := fx.store.Create(ctx, actor, wantCard)
	if err != nil {
		t.Fatal(err)
	}
	read, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if read.AudienceProblemJudgment != wantCard.AudienceProblemJudgment ||
		read.IPFit != wantCard.IPFit || read.Timing != wantCard.Timing ||
		read.ExistingContentRelation != wantCard.ExistingContentRelation ||
		read.EvidenceGapsAndInvestment != wantCard.EvidenceGapsAndInvestment ||
		!reflect.DeepEqual(read.Channels, wantCard.Channels) ||
		read.RecommendedAction != wantCard.RecommendedAction {
		t.Fatalf("the seven topic fields did not round trip:\n got %#v\nwant %#v", read, wantCard)
	}

	for _, tc := range []struct {
		action Action
		status Status
	}{
		{ActionSave, StatusSaved},
		{ActionDefer, StatusDeferred},
		{ActionDrop, StatusDropped},
	} {
		result, actionErr := fx.store.Act(ctx, workspace, actor, created.TopicCardID,
			ActionRequest{Action: tc.action, Reason: "not-now", Note: "operator note"})
		if actionErr != nil {
			t.Fatalf("%s: %v", tc.action, actionErr)
		}
		persisted, getErr := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
		if getErr != nil {
			t.Fatalf("read after %s: %v", tc.action, getErr)
		}
		if result.TopicCard.Status != tc.status || persisted.Status != tc.status {
			t.Errorf("%s status returned=%s persisted=%s", tc.action, result.TopicCard.Status, persisted.Status)
		}
		if tc.status == StatusDeferred || tc.status == StatusDropped {
			if persisted.DecisionReason != "not-now" || persisted.DecisionNote != "operator note" {
				t.Errorf("%s lost decision context", tc.action)
			}
		} else if persisted.DecisionReason != "" || persisted.DecisionNote != "" {
			t.Errorf("%s retained decision context: %#v", tc.action, persisted)
		}
	}

	started, err := fx.store.Act(ctx, workspace, actor, created.TopicCardID,
		ActionRequest{Action: ActionStart, Brief: completeBrief()})
	if err != nil {
		t.Fatal(err)
	}
	persistedStarted, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if started.TopicCard.Status != StatusStarted || persistedStarted.Status != StatusStarted ||
		persistedStarted.StartedBriefRevisionID == nil {
		t.Fatalf("start state returned=%#v persisted=%#v", started.TopicCard, persistedStarted)
	}
	startedAgain, err := fx.store.Act(ctx, workspace, actor, created.TopicCardID,
		ActionRequest{Action: ActionStart, Brief: BriefRevision{Audience: "must not replace"}})
	if err != nil {
		t.Fatal(err)
	}
	if started.Brief == nil || startedAgain.Brief == nil ||
		started.Brief.BriefRevisionID != startedAgain.Brief.BriefRevisionID {
		t.Fatalf("repeated start created a different first revision: %#v / %#v", started.Brief, startedAgain.Brief)
	}
	briefs, err := fx.store.ListBriefs(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(briefs) != 1 {
		t.Fatalf("repeated start produced %d revisions, want 1", len(briefs))
	}

	nextInput := completeBrief()
	nextInput.CoreProblem = "第二版问题"
	next, err := fx.store.AppendBrief(ctx, workspace, actor, created.TopicCardID, nextInput)
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != 2 {
		t.Fatalf("next revision = %d", next.Revision)
	}
	first, err := fx.store.GetBrief(ctx, workspace, actor, created.TopicCardID, started.Brief.BriefRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if first.CoreProblem != "选题失焦" {
		t.Fatalf("first revision was rewritten: %q", first.CoreProblem)
	}
}

func TestTopicCardFieldsAllowHonestNoneValues(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-none-values", "actor-a"
	want := TopicCard{
		WorkspaceID:               workspace,
		AudienceProblemJudgment:   "没有明确受众、问题或判断",
		IPFit:                     "没有适配依据",
		Timing:                    "没有时效依据",
		ExistingContentRelation:   "没有既有作品",
		EvidenceGapsAndInvestment: "没有现成证据或额外投入",
		Channels:                  []string{},
		RecommendedAction:         "没有推荐动作",
	}
	created, err := fx.store.Create(ctx, actor, want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AudienceProblemJudgment != want.AudienceProblemJudgment ||
		got.IPFit != want.IPFit || got.Timing != want.Timing ||
		got.ExistingContentRelation != want.ExistingContentRelation ||
		got.EvidenceGapsAndInvestment != want.EvidenceGapsAndInvestment ||
		!reflect.DeepEqual(got.Channels, want.Channels) ||
		got.RecommendedAction != want.RecommendedAction {
		t.Fatalf("honest none-values did not round trip:\n got %#v\nwant %#v", got, want)
	}
}

func TestAppendBriefRequiresTheStartAction(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-draft", "actor-a"
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.AppendBrief(ctx, workspace, actor, created.TopicCardID, completeBrief()); !errors.Is(err, ErrInvalid) {
		t.Fatalf("append before start = %v, want ErrInvalid", err)
	}
	if got := fx.db.Count(t, `SELECT count(*) FROM content_brief_revision WHERE topic_card_id=$1`, created.TopicCardID); got != 0 {
		t.Fatalf("append before start created %d revisions", got)
	}
}

// Two real appends racing for the card lock end up with different revisions.
// This is the shape the product has, not the regression guard for the lock
// ordering: which writer reaches the statement first is up to the scheduler,
// and this test passes even against the single-statement version that numbers
// from a stale snapshot. The guard is
// TestAppendBriefNumbersTheRevisionAfterItHoldsTheCardLock, which orders the
// two writers by hand (Issue #109).
func TestConcurrentBriefAppendsReceiveDistinctRevisions(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-concurrent", "actor-a"
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.Act(ctx, workspace, actor, created.TopicCardID,
		ActionRequest{Action: ActionStart, Brief: completeBrief()}); err != nil {
		t.Fatal(err)
	}

	type outcome struct {
		brief BriefRevision
		err   error
	}
	ready := make(chan struct{})
	outcomes := make(chan outcome, 2)
	var writers sync.WaitGroup
	for _, problem := range []string{"concurrent-a", "concurrent-b"} {
		writers.Go(func() {
			<-ready
			input := completeBrief()
			input.CoreProblem = problem
			brief, appendErr := fx.store.AppendBrief(ctx, workspace, actor, created.TopicCardID, input)
			outcomes <- outcome{brief: brief, err: appendErr}
		})
	}
	close(ready)
	writers.Wait()
	close(outcomes)

	revisions := map[int64]bool{}
	for result := range outcomes {
		if result.err != nil {
			t.Fatalf("concurrent append: %v", result.err)
		}
		revisions[result.brief.Revision] = true
	}
	if !revisions[2] || !revisions[3] || len(revisions) != 2 {
		t.Fatalf("concurrent revisions = %v, want 2 and 3", revisions)
	}
	briefs, err := fx.store.ListBriefs(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(briefs) != 3 {
		t.Fatalf("concurrent append left %d revisions, want 3", len(briefs))
	}
}

func TestDecisionActionsDoNotTouchAccountConfiguration(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor, account = "workspace-account", "actor-a", "account-a"
	fx.db.InsertNoID(t, "content_account", testutil.Cols{
		"account_id": account, "workspace_id": workspace, "platform": "zhihu", "display_name": "Account",
		"settings": testutil.Raw(`'{"persona":"exact","scope":"local"}'::jsonb`),
	}, "account_id=$1", account)
	card := completeCard(workspace)
	card.AccountID = new(account)
	created, err := fx.store.Create(ctx, actor, card)
	if err != nil {
		t.Fatal(err)
	}
	var before []byte
	fx.db.QueryRow(t, `SELECT to_jsonb(content_account) FROM content_account WHERE account_id=$1`, account).Scan(&before)
	for _, action := range []Action{ActionDefer, ActionDrop} {
		if _, err = fx.store.Act(ctx, workspace, actor, created.TopicCardID,
			ActionRequest{Action: action, Reason: "later", Note: "unchanged account"}); err != nil {
			t.Fatal(err)
		}
	}
	var after []byte
	fx.db.QueryRow(t, `SELECT to_jsonb(content_account) FROM content_account WHERE account_id=$1`, account).Scan(&after)
	if string(before) != string(after) {
		t.Fatalf("account changed:\nbefore %s\nafter  %s", before, after)
	}
}

func TestCreateRejectsAnAccountFromAnotherWorkspace(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const account = "foreign-account"
	fx.db.InsertNoID(t, "content_account", testutil.Cols{
		"account_id": account, "workspace_id": "workspace-owner", "platform": "zhihu", "display_name": "Foreign",
		"settings": testutil.Raw(`'{}'::jsonb`),
	}, "account_id=$1", account)
	card := completeCard("workspace-caller")
	card.AccountID = new(account)
	if _, err := fx.store.Create(ctx, "actor-a", card); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign account create = %v, want ErrNotFound", err)
	}
	count := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id='workspace-caller'`)
	if count != 0 {
		t.Fatalf("foreign account reference created %d topic cards", count)
	}
}

// fenceRecordingGuard records whether the workspace delete fence has been
// taken, and can refuse it the way workspace-core does once the workspace's
// deletion has committed.
type fenceRecordingGuard struct {
	held   bool
	denied bool
}

func (g *fenceRecordingGuard) LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error {
	if g.denied {
		return diagnostics.ErrDenied
	}
	g.held = true
	return nil
}

// fenceCheckingAccounts records, for every account read, whether the fence was
// already held when the read ran.
type fenceCheckingAccounts struct {
	testAccountReader
	guard    *fenceRecordingGuard
	reads    int
	unfenced int
}

func (a *fenceCheckingAccounts) Get(ctx context.Context, workspaceID, accountID string) (ipprofile.Account, error) {
	a.reads++
	if !a.guard.held {
		a.unfenced++
	}
	return a.testAccountReader.Get(ctx, workspaceID, accountID)
}

// Create checks the card's account inside the workspace delete fence, as
// SetAccount and marketing node adoption do: the account is read only after
// the fence is held, and not at all when the fence refuses because the
// workspace is gone. A check taken before the fence could be answered before a
// workspace deletion (or an account removal) commits and the insert applied
// after it.
func TestCreateChecksTheAccountInsideTheWorkspaceFence(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, account = "workspace-create-fence", "account-create-fence"
	fx.db.InsertNoID(t, "content_account", testutil.Cols{
		"account_id": account, "workspace_id": workspace, "platform": "zhihu", "display_name": "Fenced",
		"settings": testutil.Raw(`'{}'::jsonb`),
	}, "account_id=$1", account)
	guard := &fenceRecordingGuard{}
	accounts := &fenceCheckingAccounts{testAccountReader: fx.store.Accounts.(testAccountReader), guard: guard}
	fx.store.Guard = guard
	fx.store.Accounts = accounts

	card := completeCard(workspace)
	card.AccountID = new(account)
	created, err := fx.store.Create(ctx, "actor-a", card)
	if err != nil {
		t.Fatalf("create with this brand's account: %v", err)
	}
	if created.AccountID == nil || *created.AccountID != account {
		t.Fatalf("created account = %v, want %s", created.AccountID, account)
	}
	if accounts.reads != 1 || accounts.unfenced != 0 {
		t.Fatalf("account reads = %d, %d of them before the fence; want 1 and 0", accounts.reads, accounts.unfenced)
	}

	// The workspace's deletion has committed: the fence refuses, and the
	// account must not have been consulted at all.
	*guard = fenceRecordingGuard{denied: true}
	accounts.reads, accounts.unfenced = 0, 0
	if _, err = fx.store.Create(ctx, "actor-a", card); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create after the workspace deletion = %v, want ErrNotFound", err)
	}
	if accounts.reads != 0 {
		t.Fatalf("account read %d times although the fence refused, want 0", accounts.reads)
	}
	count := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id=$1`, workspace)
	if count != 1 {
		t.Fatalf("content_topic_card = %d, want only the card created before the deletion", count)
	}
}

func TestAuditFailureRollsBackTheTopicWrite(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	auditID := diagnostics.NewID()
	fx.db.InsertNoID(t, "content_operation_audit", testutil.Cols{
		"event_id": auditID, "workspace_id": "workspace-failure", "payload": testutil.Raw(`'{}'::jsonb`),
	}, "event_id=$1", auditID)
	ids := []string{diagnostics.NewID(), auditID, diagnostics.NewID()}
	index := 0
	fx.store.NewID = func() string {
		id := ids[index]
		index++
		return id
	}
	card := completeCard("workspace-failure")
	_, err := fx.store.Create(ctx, "actor-a", card)
	if err == nil {
		t.Fatal("create succeeded even though its audit insert failed")
	}
	count := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE topic_card_id=$1`, ids[0])
	if count != 0 {
		t.Fatalf("audit failure left %d business rows", count)
	}
}

func TestBriefStoreHasNoUpdateOrDeletePath(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	source, err := os.ReadFile(filepath.Join(filepath.Dir(current), "store.go"))
	if err != nil {
		t.Fatal(err)
	}
	upper := strings.ToUpper(string(source))
	for _, forbidden := range []string{"UPDATE CONTENT_BRIEF_REVISION", "DELETE FROM CONTENT_BRIEF_REVISION"} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("append-only store contains forbidden path %q", forbidden)
		}
	}
	if !strings.Contains(upper, "INSERT INTO CONTENT_BRIEF_REVISION") {
		t.Fatal("append-only store has no insert path")
	}
}

func TestFixtureSchemaNameIsSafe(t *testing.T) {
	name := "topic_test_" + diagnostics.NewID()
	if strings.ContainsAny(name, `";' `) {
		t.Fatalf("unsafe schema name %q", name)
	}
	if !strings.HasPrefix(name, "topic_test_") || len(name) != len("topic_test_")+32 {
		t.Fatalf("unexpected schema name %q", name)
	}
}

// waitForCardLockWaiter blocks until some other backend is waiting on a lock in
// a statement that reads content_topic_card, which is how this process observes
// that the appending goroutine has started its statement - and therefore taken
// its snapshot - while the test still holds the card lock. Without this the
// orchestration would race: an append that only starts after the competing
// revision is committed sees it, and the stale-snapshot defect stays hidden.
func waitForCardLockWaiter(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := pool.QueryRow(context.Background(), `
			SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND pid <> pg_backend_pid()
			AND wait_event_type = 'Lock' AND query LIKE '%content_topic_card%'`).Scan(&waiting); err != nil {
			t.Fatalf("inspect lock waiters: %v", err)
		}
		if waiting > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no backend ever waited on the topic card lock; the orchestration proved nothing")
}

// The revision number must be computed after the card lock is held, not in the
// same statement that acquires it. READ COMMITTED gives a statement one
// snapshot, taken when the statement starts; an appender that waits for the
// lock still evaluates max(revision) against that older snapshot and picks a
// number the winner has already taken, so a legitimate save dies on the unique
// index. The orchestration below forces exactly that order: the appender starts
// and blocks, the competing revision commits, and only then does the appender
// get the lock.
func TestAppendBriefNumbersTheRevisionAfterItHoldsTheCardLock(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-lock-order", "actor-a"
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.Act(ctx, workspace, actor, created.TopicCardID,
		ActionRequest{Action: ActionStart, Brief: completeBrief()}); err != nil {
		t.Fatal(err)
	}

	// The winner holds the card lock and commits its own revision 2 without
	// letting go in between, so the appender below can only resume after that
	// revision is visible to any snapshot taken from now on.
	winner, err := fx.db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer winner.Rollback(ctx)
	var locked string
	if err = winner.QueryRow(ctx, `SELECT topic_card_id FROM content_topic_card
		WHERE workspace_id=$1 AND topic_card_id=$2 FOR UPDATE`,
		workspace, created.TopicCardID).Scan(&locked); err != nil {
		t.Fatalf("take the card lock: %v", err)
	}

	appended := make(chan struct {
		brief BriefRevision
		err   error
	}, 1)
	go func() {
		input := completeBrief()
		input.CoreProblem = "waited for the lock"
		brief, appendErr := fx.store.AppendBrief(ctx, workspace, actor, created.TopicCardID, input)
		appended <- struct {
			brief BriefRevision
			err   error
		}{brief, appendErr}
	}()
	waitForCardLockWaiter(t, fx.db.Pool)

	if _, err = winner.Exec(ctx, `INSERT INTO content_brief_revision (
		brief_revision_id, topic_card_id, workspace_id, revision, audience,
		core_problem, claim_and_boundaries, channels, format, structure,
		citation_requirements, source_scope, deliverable, time_limit, cost_limit
	) VALUES ($1,$2,$3,2,'a','b','c','[]'::jsonb,'d','e','f','g','h','i','j')`,
		"winner-"+created.TopicCardID, created.TopicCardID, workspace); err != nil {
		t.Fatalf("winner writes revision 2: %v", err)
	}
	if err = winner.Commit(ctx); err != nil {
		t.Fatalf("winner commit: %v", err)
	}

	result := <-appended
	if result.err != nil {
		t.Fatalf("append after waiting for the lock: %v", result.err)
	}
	if result.brief.Revision != 3 {
		t.Fatalf("append after waiting numbered itself %d, want 3", result.brief.Revision)
	}
	briefs, err := fx.store.ListBriefs(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	revisions := []int64{}
	for _, brief := range briefs {
		revisions = append(revisions, brief.Revision)
	}
	if !reflect.DeepEqual(revisions, []int64{1, 2, 3}) {
		t.Fatalf("revisions = %v, want [1 2 3]", revisions)
	}
}

// The start path numbers the first revision from a constant rather than from a
// count, so it cannot pick a stale number the way AppendBrief could. What it
// does read under the lock is started_brief_revision_id, and that read has to
// see the winner's value: a start that still believed the card had no first
// version would insert a second revision 1 and die on the same unique index.
// This is the start half of Issue #109, orchestrated the same way.
func TestStartAfterWaitingForTheCardLockReturnsTheWinnersFirstBrief(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-start-lock-order", "actor-a"
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}

	winner, err := fx.db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer winner.Rollback(ctx)
	var locked string
	if err = winner.QueryRow(ctx, `SELECT topic_card_id FROM content_topic_card
		WHERE workspace_id=$1 AND topic_card_id=$2 FOR UPDATE`,
		workspace, created.TopicCardID).Scan(&locked); err != nil {
		t.Fatalf("take the card lock: %v", err)
	}

	started := make(chan struct {
		result ActionResult
		err    error
	}, 1)
	go func() {
		result, actErr := fx.store.Act(ctx, workspace, actor, created.TopicCardID,
			ActionRequest{Action: ActionStart, Brief: completeBrief()})
		started <- struct {
			result ActionResult
			err    error
		}{result, actErr}
	}()
	waitForCardLockWaiter(t, fx.db.Pool)

	// Exactly what a start that won the lock commits: the first revision plus
	// the pointer to it on the card.
	winnerRevisionID := "winner-first-" + created.TopicCardID
	if _, err = winner.Exec(ctx, `INSERT INTO content_brief_revision (
		brief_revision_id, topic_card_id, workspace_id, revision, audience,
		core_problem, claim_and_boundaries, channels, format, structure,
		citation_requirements, source_scope, deliverable, time_limit, cost_limit
	) VALUES ($1,$2,$3,1,'a','b','c','[]'::jsonb,'d','e','f','g','h','i','j')`,
		winnerRevisionID, created.TopicCardID, workspace); err != nil {
		t.Fatalf("winner writes the first revision: %v", err)
	}
	if _, err = winner.Exec(ctx, `UPDATE content_topic_card
		SET status='started', started_brief_revision_id=$3, updated_at=now()
		WHERE workspace_id=$1 AND topic_card_id=$2`,
		workspace, created.TopicCardID, winnerRevisionID); err != nil {
		t.Fatalf("winner points the card at it: %v", err)
	}
	if err = winner.Commit(ctx); err != nil {
		t.Fatalf("winner commit: %v", err)
	}

	outcome := <-started
	if outcome.err != nil {
		t.Fatalf("start after waiting for the lock: %v", outcome.err)
	}
	if outcome.result.Brief == nil || outcome.result.Brief.BriefRevisionID != winnerRevisionID {
		t.Fatalf("start after waiting returned %+v, want the winner's %s",
			outcome.result.Brief, winnerRevisionID)
	}
	briefs, err := fx.store.ListBriefs(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(briefs) != 1 {
		t.Fatalf("two starts left %d revisions, want 1", len(briefs))
	}
}

// Attaching a card to an account, moving it to another, and detaching it
// again. The link is what makes "why this fits this IP" a question about a
// particular account rather than about the brand in general (SOP §5.2).
func TestTopicCardAccountLinkRoundTrip(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-link", "actor-a"
	for _, account := range []string{"account-one", "account-two"} {
		fx.db.InsertNoID(t, "content_account", testutil.Cols{
			"account_id": account, "workspace_id": workspace, "platform": "zhihu",
			"display_name": account, "settings": testutil.Raw(`'{}'::jsonb`),
		}, "account_id=$1", account)
	}
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if created.AccountID != nil {
		t.Fatalf("a new card starts linked to %v, want nil", created.AccountID)
	}

	first := "account-one"
	linked, err := fx.store.SetAccount(ctx, workspace, actor, created.TopicCardID, &first)
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if linked.AccountID == nil || *linked.AccountID != first {
		t.Fatalf("linked card = %v, want %s", linked.AccountID, first)
	}
	second := "account-two"
	if _, err = fx.store.SetAccount(ctx, workspace, actor, created.TopicCardID, &second); err != nil {
		t.Fatalf("relink: %v", err)
	}
	detached, err := fx.store.SetAccount(ctx, workspace, actor, created.TopicCardID, nil)
	if err != nil {
		t.Fatalf("detach: %v", err)
	}
	if detached.AccountID != nil {
		t.Fatalf("detached card = %v, want nil", detached.AccountID)
	}
	// Read back rather than trust the returned struct: the row is what the
	// next request will see.
	reread, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.AccountID != nil {
		t.Fatalf("stored link = %v, want nil", reread.AccountID)
	}
	// Three links, three audit events. "Who pointed this card at that account,
	// and when" is the question the audit exists to answer.
	events := fx.db.Count(t, `SELECT count(*) FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2 AND payload->>'step'='link-account'`,
		workspace, created.TopicCardID)
	if events != 3 {
		t.Fatalf("link audit events = %d, want 3", events)
	}
}

// An account from another brand is refused exactly like a card from another
// brand, and the link the card already had is left alone.
func TestSetAccountRejectsAnAccountFromAnotherWorkspace(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-link-foreign", "actor-a"
	fx.db.InsertNoID(t, "content_account", testutil.Cols{
		"account_id": "mine", "workspace_id": workspace, "platform": "zhihu",
		"display_name": "Mine", "settings": testutil.Raw(`'{}'::jsonb`),
	}, "account_id=$1", "mine")
	fx.db.InsertNoID(t, "content_account", testutil.Cols{
		"account_id": "theirs", "workspace_id": "workspace-other", "platform": "zhihu",
		"display_name": "Theirs", "settings": testutil.Raw(`'{}'::jsonb`),
	}, "account_id=$1", "theirs")
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	mine := "mine"
	if _, err = fx.store.SetAccount(ctx, workspace, actor, created.TopicCardID, &mine); err != nil {
		t.Fatal(err)
	}
	theirs := "theirs"
	if _, err = fx.store.SetAccount(ctx, workspace, actor, created.TopicCardID, &theirs); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign account link = %v, want ErrNotFound", err)
	}
	if _, err = fx.store.SetAccount(ctx, workspace, actor, created.TopicCardID, stringPointer("missing")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account link = %v, want ErrNotFound", err)
	}
	reread, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if reread.AccountID == nil || *reread.AccountID != mine {
		t.Fatalf("refused link changed the card to %v", reread.AccountID)
	}
}

// The list narrows to one account, to the cards no account has been chosen
// for, or to everything.
func TestListFiltersByAccount(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-filter", "actor-a"
	fx.db.InsertNoID(t, "content_account", testutil.Cols{
		"account_id": "filter-account", "workspace_id": workspace, "platform": "zhihu",
		"display_name": "Filter", "settings": testutil.Raw(`'{}'::jsonb`),
	}, "account_id=$1", "filter-account")
	linkedCard, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	unlinkedCard, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	account := "filter-account"
	if _, err = fx.store.SetAccount(ctx, workspace, actor, linkedCard.TopicCardID, &account); err != nil {
		t.Fatal(err)
	}

	ids := func(filter string) []string {
		t.Helper()
		cards, listErr := fx.store.List(ctx, workspace, actor, filter)
		if listErr != nil {
			t.Fatalf("list %q: %v", filter, listErr)
		}
		out := []string{}
		for _, card := range cards {
			out = append(out, card.TopicCardID)
		}
		sort.Strings(out)
		return out
	}
	both := []string{linkedCard.TopicCardID, unlinkedCard.TopicCardID}
	sort.Strings(both)
	if got := ids(""); !reflect.DeepEqual(got, both) {
		t.Errorf("unfiltered list = %v, want both cards", got)
	}
	if got := ids(account); !reflect.DeepEqual(got, []string{linkedCard.TopicCardID}) {
		t.Errorf("list by account = %v, want the linked card", got)
	}
	if got := ids(AccountFilterNone); !reflect.DeepEqual(got, []string{unlinkedCard.TopicCardID}) {
		t.Errorf("list of unlinked = %v, want the unlinked card", got)
	}
	// A filter naming an account with no cards is an empty list, not an error
	// and not every card.
	if got := ids("filter-account-with-nothing"); len(got) != 0 {
		t.Errorf("list by an unused account = %v, want none", got)
	}
}

func stringPointer(value string) *string { return &value }

func insertTestSource(t *testing.T, fx topicFixture, workspace, actor, sourceID, status string) {
	t.Helper()
	fx.db.InsertNoID(t, "content_source", testutil.Cols{
		"source_id":    sourceID,
		"workspace_id": workspace,
		"kind":         "pasted_text",
		"recorded_by":  actor,
		"title":        "Test Source " + sourceID,
		"status":       status,
	}, "source_id=$1", sourceID)
}

func TestTopicCardSourceReferencesRoundTrip(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-sources", "actor-a"
	insertTestSource(t, fx, workspace, actor, "src-1", "inbox")
	insertTestSource(t, fx, workspace, actor, "src-2", "inbox")

	cardInput := completeCard(workspace)
	cardInput.FitSourceIDs = []string{"src-1"}
	cardInput.EvidenceSourceIDs = []string{"src-2"}

	created, err := fx.store.Create(ctx, actor, cardInput)
	if err != nil {
		t.Fatalf("Create card with sources: %v", err)
	}
	if !reflect.DeepEqual(created.FitSourceIDs, []string{"src-1"}) {
		t.Errorf("created.FitSourceIDs = %v, want [src-1]", created.FitSourceIDs)
	}
	if !reflect.DeepEqual(created.EvidenceSourceIDs, []string{"src-2"}) {
		t.Errorf("created.EvidenceSourceIDs = %v, want [src-2]", created.EvidenceSourceIDs)
	}

	fetched, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatalf("Get card: %v", err)
	}
	if !reflect.DeepEqual(fetched.FitSourceIDs, []string{"src-1"}) {
		t.Errorf("fetched.FitSourceIDs = %v, want [src-1]", fetched.FitSourceIDs)
	}
	if !reflect.DeepEqual(fetched.EvidenceSourceIDs, []string{"src-2"}) {
		t.Errorf("fetched.EvidenceSourceIDs = %v, want [src-2]", fetched.EvidenceSourceIDs)
	}

	// Update with SetSources: replace fit with ["src-2"], and clear evidence with []
	newFit := []string{"src-2"}
	clearEvidence := []string{}
	updated, err := fx.store.SetSources(ctx, workspace, actor, created.TopicCardID, &newFit, &clearEvidence)
	if err != nil {
		t.Fatalf("SetSources: %v", err)
	}
	if !reflect.DeepEqual(updated.FitSourceIDs, []string{"src-2"}) {
		t.Errorf("updated.FitSourceIDs = %v, want [src-2]", updated.FitSourceIDs)
	}
	if updated.EvidenceSourceIDs == nil || len(updated.EvidenceSourceIDs) != 0 {
		t.Errorf("updated.EvidenceSourceIDs = %v, want empty non-nil slice", updated.EvidenceSourceIDs)
	}
}

func TestSetSourcesDefaultDoesNotClear(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-default", "actor-a"
	insertTestSource(t, fx, workspace, actor, "src-1", "inbox")
	insertTestSource(t, fx, workspace, actor, "src-2", "inbox")

	cardInput := completeCard(workspace)
	cardInput.FitSourceIDs = []string{"src-1"}
	cardInput.EvidenceSourceIDs = []string{"src-2"}
	created, err := fx.store.Create(ctx, actor, cardInput)
	if err != nil {
		t.Fatal(err)
	}

	// fitSourceIDs provided as ["src-2"], evidenceSourceIDs omitted (nil).
	// Evidence column must NOT be cleared.
	newFit := []string{"src-2"}
	updated, err := fx.store.SetSources(ctx, workspace, actor, created.TopicCardID, &newFit, nil)
	if err != nil {
		t.Fatalf("SetSources: %v", err)
	}
	if !reflect.DeepEqual(updated.FitSourceIDs, []string{"src-2"}) {
		t.Errorf("updated.FitSourceIDs = %v, want [src-2]", updated.FitSourceIDs)
	}
	if !reflect.DeepEqual(updated.EvidenceSourceIDs, []string{"src-2"}) {
		t.Errorf("updated.EvidenceSourceIDs = %v, want [src-2] (should remain untouched)", updated.EvidenceSourceIDs)
	}

	// Verify status and other card fields have not changed.
	if updated.Status != created.Status {
		t.Errorf("status = %v, want %v", updated.Status, created.Status)
	}
	if updated.AudienceProblemJudgment != created.AudienceProblemJudgment {
		t.Errorf("text fields altered unexpectedly")
	}
}

func TestSetSourcesRejectsForeignOrMissingSource(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspaceA, workspaceB, actor = "workspace-a", "workspace-b", "actor-a"
	insertTestSource(t, fx, workspaceA, actor, "src-local", "inbox")
	insertTestSource(t, fx, workspaceB, actor, "src-foreign", "inbox")

	card, err := fx.store.Create(ctx, actor, completeCard(workspaceA))
	if err != nil {
		t.Fatal(err)
	}

	// 1. Foreign workspace source ID -> ErrNotFound (404)
	foreignFit := []string{"src-foreign"}
	_, err = fx.store.SetSources(ctx, workspaceA, actor, card.TopicCardID, &foreignFit, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign source err = %v, want ErrNotFound", err)
	}

	// 2. Non-existent source ID -> ErrNotFound (404)
	missingFit := []string{"src-does-not-exist"}
	_, err = fx.store.SetSources(ctx, workspaceA, actor, card.TopicCardID, &missingFit, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing source err = %v, want ErrNotFound", err)
	}

	// 3. All-or-nothing: one valid, one invalid -> whole write rejected, 0 rows written
	mixedFit := []string{"src-local", "src-does-not-exist"}
	_, err = fx.store.SetSources(ctx, workspaceA, actor, card.TopicCardID, &mixedFit, nil)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("mixed sources err = %v, want ErrNotFound", err)
	}

	// Confirm card still has 0 sources linked
	fetched, err := fx.store.Get(ctx, workspaceA, actor, card.TopicCardID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fetched.FitSourceIDs) != 0 || len(fetched.EvidenceSourceIDs) != 0 {
		t.Fatalf("card has sources after rollback: fit=%v, ev=%v", fetched.FitSourceIDs, fetched.EvidenceSourceIDs)
	}
}

func TestDecisionActionsDoNotTouchSourceReferences(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-actions", "actor-a"
	insertTestSource(t, fx, workspace, actor, "src-1", "inbox")
	insertTestSource(t, fx, workspace, actor, "src-2", "inbox")

	cardInput := completeCard(workspace)
	cardInput.FitSourceIDs = []string{"src-1"}
	cardInput.EvidenceSourceIDs = []string{"src-2"}
	card, err := fx.store.Create(ctx, actor, cardInput)
	if err != nil {
		t.Fatal(err)
	}

	for _, action := range []Action{ActionSave, ActionDefer, ActionDrop} {
		result, actErr := fx.store.Act(ctx, workspace, actor, card.TopicCardID, ActionRequest{Action: action})
		if actErr != nil {
			t.Fatalf("Act(%s): %v", action, actErr)
		}
		if !reflect.DeepEqual(result.TopicCard.FitSourceIDs, []string{"src-1"}) {
			t.Errorf("Act(%s) changed FitSourceIDs: %v", action, result.TopicCard.FitSourceIDs)
		}
		if !reflect.DeepEqual(result.TopicCard.EvidenceSourceIDs, []string{"src-2"}) {
			t.Errorf("Act(%s) changed EvidenceSourceIDs: %v", action, result.TopicCard.EvidenceSourceIDs)
		}
	}
}

func TestSetSourcesAcceptsArchivedSource(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-archived", "actor-a"
	insertTestSource(t, fx, workspace, actor, "src-archived", "archived")

	card, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}

	// Archived source can be referenced (validation only asks workspace & existence)
	archivedFit := []string{"src-archived"}
	updated, err := fx.store.SetSources(ctx, workspace, actor, card.TopicCardID, &archivedFit, nil)
	if err != nil {
		t.Fatalf("SetSources with archived source: %v", err)
	}
	if !reflect.DeepEqual(updated.FitSourceIDs, []string{"src-archived"}) {
		t.Errorf("updated.FitSourceIDs = %v, want [src-archived]", updated.FitSourceIDs)
	}
}

func TestPatchBodyChangesOnlyItsFiveFieldsAndLeavesFrozenObjectsUntouched(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	seedAccount(t, fx, string(ipprofile.ScopeAll), readyProfile())
	cardID, briefID := seedStartableBrief(t, fx)
	snapshot, err := fx.store.Start(ctx, startWorkspace, "actor-start", cardID, briefID,
		startRequest(string(ipprofile.ScopeWeb)))
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	beforeCard, err := fx.store.Get(ctx, startWorkspace, "actor-start", cardID)
	if err != nil {
		t.Fatalf("get before patch: %v", err)
	}
	beforeBrief, err := fx.store.GetBrief(ctx, startWorkspace, "actor-start", cardID, briefID)
	if err != nil {
		t.Fatalf("get brief before patch: %v", err)
	}
	beforeSnapshot, err := fx.store.GetSnapshot(ctx, startWorkspace, "actor-start", cardID, snapshot.SnapshotID)
	if err != nil {
		t.Fatalf("get snapshot before patch: %v", err)
	}

	updated, err := fx.store.PatchBody(ctx, startWorkspace, "actor-start", cardID, TopicBodyPatch{
		IPFit:                     PatchString{Set: true, Value: ""},
		Timing:                    PatchString{Set: true, Value: "publish after the next interview"},
		EvidenceGapsAndInvestment: PatchString{Set: true, Value: "need one primary source"},
	})
	if err != nil {
		t.Fatalf("patch body: %v", err)
	}
	if updated.IPFit != "" || updated.Timing != "publish after the next interview" ||
		updated.EvidenceGapsAndInvestment != "need one primary source" {
		t.Fatalf("updated editable fields = %#v", updated)
	}
	if updated.AudienceProblemJudgment != beforeCard.AudienceProblemJudgment ||
		updated.ExistingContentRelation != beforeCard.ExistingContentRelation ||
		!reflect.DeepEqual(updated.Channels, beforeCard.Channels) ||
		updated.RecommendedAction != beforeCard.RecommendedAction ||
		updated.Status != beforeCard.Status ||
		updated.DecisionReason != beforeCard.DecisionReason ||
		updated.DecisionNote != beforeCard.DecisionNote ||
		!reflect.DeepEqual(updated.AccountID, beforeCard.AccountID) ||
		!reflect.DeepEqual(updated.FitSourceIDs, beforeCard.FitSourceIDs) ||
		!reflect.DeepEqual(updated.EvidenceSourceIDs, beforeCard.EvidenceSourceIDs) ||
		!reflect.DeepEqual(updated.StartedBriefRevisionID, beforeCard.StartedBriefRevisionID) {
		t.Fatalf("body patch changed an out-of-scope card field: before=%#v after=%#v", beforeCard, updated)
	}

	afterBrief, err := fx.store.GetBrief(ctx, startWorkspace, "actor-start", cardID, briefID)
	if err != nil {
		t.Fatalf("get brief after patch: %v", err)
	}
	afterSnapshot, err := fx.store.GetSnapshot(ctx, startWorkspace, "actor-start", cardID, snapshot.SnapshotID)
	if err != nil {
		t.Fatalf("get snapshot after patch: %v", err)
	}
	if !reflect.DeepEqual(afterBrief, beforeBrief) {
		t.Fatalf("body patch changed frozen brief: before=%#v after=%#v", beforeBrief, afterBrief)
	}
	if !reflect.DeepEqual(afterSnapshot, beforeSnapshot) {
		t.Fatalf("body patch changed start snapshot: before=%#v after=%#v", beforeSnapshot, afterSnapshot)
	}
	if events := fx.db.Count(t, `SELECT count(*) FROM content_operation_audit
		WHERE workspace_id=$1 AND payload->>'object_id'=$2 AND payload->>'step'='edit-body'`,
		startWorkspace, cardID); events != 1 {
		t.Fatalf("edit-body audit events = %d, want 1", events)
	}
}

func TestPatchBodyAuditFailureRollsBackCardWrite(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-body-audit", "actor-body-audit"
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatalf("create card: %v", err)
	}
	before, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatalf("get card before failed patch: %v", err)
	}

	auditID := diagnostics.NewID()
	fx.db.InsertNoID(t, "content_operation_audit", testutil.Cols{
		"event_id": auditID, "workspace_id": workspace, "payload": testutil.Raw(`'{}'::jsonb`),
	}, "event_id=$1", auditID)
	ids := []string{auditID, diagnostics.NewID()}
	index := 0
	fx.store.NewID = func() string {
		id := ids[index]
		index++
		return id
	}

	_, err = fx.store.PatchBody(ctx, workspace, actor, created.TopicCardID,
		TopicBodyPatch{Timing: PatchString{Set: true, Value: "must not persist"}})
	if err == nil {
		t.Fatal("body patch succeeded even though its audit insert failed")
	}
	after, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatalf("get card after failed patch: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("audit failure changed card: before=%#v after=%#v", before, after)
	}
	if events := fx.db.Count(t, `SELECT count(*) FROM content_operation_audit WHERE event_id=$1`, auditID); events != 1 {
		t.Fatalf("audit collision count = %d, want original event only", events)
	}
}

func TestPatchBodyConcurrentDifferentFieldsDoNotOverwriteEachOther(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-body-concurrent", "actor-body"
	created, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var group sync.WaitGroup
	group.Go(func() {
		<-start
		_, patchErr := fx.store.PatchBody(ctx, workspace, actor, created.TopicCardID,
			TopicBodyPatch{AudienceProblemJudgment: PatchString{Set: true, Value: "updated audience"}})
		errs <- patchErr
	})
	group.Go(func() {
		<-start
		_, patchErr := fx.store.PatchBody(ctx, workspace, actor, created.TopicCardID,
			TopicBodyPatch{Timing: PatchString{Set: true, Value: "updated timing"}})
		errs <- patchErr
	})
	close(start)
	group.Wait()
	close(errs)
	for patchErr := range errs {
		if patchErr != nil {
			t.Fatalf("concurrent body patch: %v", patchErr)
		}
	}

	stored, err := fx.store.Get(ctx, workspace, actor, created.TopicCardID)
	if err != nil {
		t.Fatalf("get after concurrent patches: %v", err)
	}
	if stored.AudienceProblemJudgment != "updated audience" || stored.Timing != "updated timing" {
		t.Fatalf("concurrent patches lost an edit: %#v", stored)
	}
}

// ---------------------------------------------------------------- CreateOnce

// originKeyOf reads the stored origin key of a card; "" is NULL.
func originKeyOf(t *testing.T, fx topicFixture, topicCardID string) string {
	t.Helper()
	var key *string
	fx.db.QueryRow(t, `SELECT origin_key FROM content_topic_card WHERE topic_card_id=$1`, topicCardID).Scan(&key)
	if key == nil {
		return ""
	}
	return *key
}

// T068a (specs/035 contract §7.4): the same key twice is one card; the
// second call answers that card with created false and writes nothing. A
// different key is another card.
func TestCreateOnceWritesOneCardPerKey(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor, key = "workspace-create-once", "actor-once", "opdiag-suggestion:s1"
	first, created, err := fx.store.CreateOnce(ctx, actor, completeCard(workspace), key)
	if err != nil || !created {
		t.Fatalf("first CreateOnce = created %v, %v", created, err)
	}
	if first.Status != StatusDraft || first.IPFit != completeCard(workspace).IPFit {
		t.Fatalf("first card = %#v", first)
	}
	again := completeCard(workspace)
	again.IPFit = "a different body"
	second, created, err := fx.store.CreateOnce(ctx, actor, again, key)
	if err != nil || created {
		t.Fatalf("second CreateOnce = created %v, %v", created, err)
	}
	if second.TopicCardID != first.TopicCardID || second.IPFit != first.IPFit {
		t.Fatalf("second answered %s (%q), want the first card %s", second.TopicCardID, second.IPFit, first.TopicCardID)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id=$1`, workspace); n != 1 {
		t.Fatalf("%d cards after two calls with one key, want 1", n)
	}
	if got := originKeyOf(t, fx, first.TopicCardID); got != key {
		t.Fatalf("origin key = %q, want %q", got, key)
	}
	other, created, err := fx.store.CreateOnce(ctx, actor, completeCard(workspace), "opdiag-suggestion:s2")
	if err != nil || !created || other.TopicCardID == first.TopicCardID {
		t.Fatalf("another key = %s created %v, %v", other.TopicCardID, created, err)
	}
	// The key is per workspace.
	elsewhere, created, err := fx.store.CreateOnce(ctx, actor, completeCard("workspace-create-once-b"), key)
	if err != nil || !created || elsewhere.TopicCardID == first.TopicCardID {
		t.Fatalf("same key in another workspace = %s created %v, %v", elsewhere.TopicCardID, created, err)
	}
}

// T068a: two transactions with the same key at once end with exactly one
// card, and both callers get it. The first writer holds its insert open, so
// the CreateOnce behind it passes its read, waits on the unique index, and
// then has to answer the card the first one committed.
func TestCreateOnceRacingTheSameKeyAnswersTheWinnersCard(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor, key = "workspace-create-once-race", "actor-once", "opdiag-suggestion:race"
	tx, err := fx.db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	winner, err := fx.store.insertCardTx(ctx, tx, func() TopicCard {
		card, _ := prepareNewCard(completeCard(workspace))
		card.TopicCardID = "card-winner"
		return card
	}(), key)
	if err != nil {
		t.Fatalf("hold the first insert: %v", err)
	}
	type answer struct {
		card    TopicCard
		created bool
		err     error
	}
	done := make(chan answer, 1)
	go func() {
		card, created, callErr := fx.store.CreateOnce(ctx, actor, completeCard(workspace), key)
		done <- answer{card, created, callErr}
	}()
	// Give the second call time to pass its read and block on the index.
	time.Sleep(300 * time.Millisecond)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil || got.created || got.card.TopicCardID != winner.TopicCardID {
		t.Fatalf("the racing call = %s created %v, %v; want the winner %s", got.card.TopicCardID, got.created,
			got.err, winner.TopicCardID)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id=$1`, workspace); n != 1 {
		t.Fatalf("%d cards after a race on one key, want 1", n)
	}

	// And many at once, with nothing held: still one card, the same for all.
	const racers = 8
	ids := make([]string, racers)
	errs := make([]error, racers)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := range racers {
		group.Go(func() {
			<-start
			card, _, callErr := fx.store.CreateOnce(ctx, actor, completeCard(workspace), "opdiag-suggestion:many")
			ids[i], errs[i] = card.TopicCardID, callErr
		})
	}
	close(start)
	group.Wait()
	for i := range racers {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("racer %d = %q, %v; racer 0 = %q", i, ids[i], errs[i], ids[0])
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id=$1 AND origin_key=$2`,
		workspace, "opdiag-suggestion:many"); n != 1 {
		t.Fatalf("%d cards for one key after %d racers, want 1", n, racers)
	}
}

// T068a: an empty key is refused and writes nothing; Create never writes a
// key, so every card it made before or after this card is untouched.
func TestCreateOnceNeedsAKeyAndCreateNeverWritesOne(t *testing.T) {
	fx := newTopicFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-create-once-empty", "actor-once"
	for _, key := range []string{"", "   "} {
		if _, _, err := fx.store.CreateOnce(ctx, actor, completeCard(workspace), key); !errors.Is(err, ErrInvalid) {
			t.Fatalf("key %q = %v, want ErrInvalid", key, err)
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id=$1`, workspace); n != 0 {
		t.Fatalf("%d cards after refused keys", n)
	}
	plain, err := fx.store.Create(ctx, actor, completeCard(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if got := originKeyOf(t, fx, plain.TopicCardID); got != "" {
		t.Fatalf("Create wrote origin key %q", got)
	}
	// Two plain cards side by side: NULL keys never collide.
	if _, err = fx.store.Create(ctx, actor, completeCard(workspace)); err != nil {
		t.Fatalf("a second plain card: %v", err)
	}
	// The response shape is unchanged: no origin key in a card's JSON.
	keyed, _, err := fx.store.CreateOnce(ctx, actor, completeCard(workspace), "opdiag-suggestion:json")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(keyed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "origin") {
		t.Fatalf("a card's JSON carries the origin key: %s", encoded)
	}
}
