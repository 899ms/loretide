package topicplanning

import (
	"context"
	"errors"
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
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	"github.com/multica-ai/multica/server/internal/testutil"
)

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
	err := r.pool.QueryRow(ctx, `SELECT account_id, workspace_id FROM content_account
		WHERE workspace_id=$1 AND account_id=$2`, workspaceID, accountID).
		Scan(&account.AccountID, &account.WorkspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ipprofile.Account{}, ipprofile.ErrNotFound
	}
	return account, err
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
		"483_content_topic_card.up.sql",
		"485_content_brief_revision.up.sql",
		"486_content_brief_revision_unique_idx.up.sql",
		"488_content_topic_card_id_unique_idx.up.sql",
		"489_content_brief_revision_id_unique_idx.up.sql",
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
		store: &Store{DB: pool, Diagnostics: diagnosticStore, Accounts: testAccountReader{pool}, Build: "test"},
		db:    testutil.New(pool, "", ""),
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
