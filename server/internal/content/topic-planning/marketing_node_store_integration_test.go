package topicplanning

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// Real PostgreSQL tests for the marketing node store (specs/033 PR 1: T017,
// T018, T019). They run through scripts/test-go-db.sh --suite topic-planning
// and skip without its database, like the rest of this package's fixture.

// newNodeFixture is the topic fixture plus the marketing node migrations.
// The files are found by name rather than by number: the numbers are
// provisional until merge (the node PR and a parallel one both start after
// app-main's highest migration), and a renumbering must not break this list.
func newNodeFixture(t *testing.T) topicFixture {
	t.Helper()
	fx := newTopicFixture(t)
	_, current, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(current), "..", "..", "..", "migrations")
	matches, err := filepath.Glob(filepath.Join(dir, "*_content_marketing_node*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 9 {
		t.Fatalf("found %d marketing node migrations, want 9", len(matches))
	}
	number := func(path string) int {
		n, _ := strconv.Atoi(strings.SplitN(filepath.Base(path), "_", 2)[0])
		return n
	}
	sort.Slice(matches, func(i, j int) bool { return number(matches[i]) < number(matches[j]) })
	for _, path := range matches {
		sql, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		fx.db.Exec(t, string(sql))
	}
	fx.store.Now = func() time.Time { return time.Date(2026, 11, 10, 16, 30, 0, 0, time.UTC) }
	return fx
}

func nodeRequest(name, startsOn string) CreateNodeRequest {
	return CreateNodeRequest{Content: NodeContent{
		Name: name, Kind: NodeKindMarketing, StartsOn: startsOn, EndsOn: startsOn,
		Timezone: "Asia/Shanghai", LeadDays: lead(14), Goal: "清库存 + 拉新",
		DateCertainty: DateConfirmed,
	}}
}

func seedNodeAccount(t *testing.T, fx topicFixture, workspace, accountID string) {
	t.Helper()
	fx.db.InsertNoID(t, "content_account", testutil.Cols{
		"account_id": accountID, "workspace_id": workspace, "platform": "zhihu",
		"display_name": "账号 " + accountID, "settings": testutil.Raw(`'{}'::jsonb`),
	}, "account_id=$1", accountID)
}

func TestNodeCreateReadAndReviseKeepEveryRevision(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-node", "actor-a"
	seedNodeAccount(t, fx, workspace, "acct-node-1")
	insertTestSource(t, fx, workspace, actor, "src-node-1", "inbox")

	req := nodeRequest(" 双十一 ", "2026-11-11")
	req.Content.Accounts = []NodeAccount{{AccountID: "acct-node-1", Role: "主推"}}
	req.Content.MaterialSourceIDs = []string{"src-node-1"}
	req.Note = "首版"
	created, err := fx.store.CreateNode(ctx, workspace, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if created.Status != NodeStatusActive || created.Origin != NodeOriginManual ||
		created.CurrentRevision != 1 || created.Current.ChangeKind != ChangeCreate {
		t.Fatalf("created = %+v", created)
	}
	if created.Current.Name != "双十一" {
		t.Fatalf("stored name %q, want it trimmed", created.Current.Name)
	}
	// 16:30Z on 11-10 is 00:30 on 11-11 in Shanghai: the node is live.
	if created.Timing.Today != "2026-11-11" || created.Timing.Phase != PhaseLive {
		t.Fatalf("timing = %+v", created.Timing)
	}

	read, err := fx.store.GetNode(ctx, workspace, actor, created.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read.Current.NodeContent, created.Current.NodeContent) ||
		read.Current.Note != "首版" || read.Current.Actor != actor ||
		read.Current.RevisionID != created.Current.RevisionID {
		t.Fatalf("read back %+v\nwant %+v", read.Current, created.Current)
	}

	edited := req.Content
	edited.Goal = "只拉新"
	revised, err := fx.store.ReviseNode(ctx, workspace, actor, created.NodeID,
		ReviseNodeRequest{BaseRevision: 1, Content: edited, Note: "改目标"})
	if err != nil {
		t.Fatal(err)
	}
	if revised.CurrentRevision != 2 || revised.Current.ChangeKind != ChangeEdit ||
		revised.Current.Goal != "只拉新" || revised.Status != NodeStatusActive {
		t.Fatalf("revised = %+v", revised)
	}

	history, err := fx.store.ListNodeRevisions(ctx, workspace, actor, created.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0].Revision != 1 || history[1].Revision != 2 {
		t.Fatalf("history = %+v", history)
	}
	first := history[0]
	if !reflect.DeepEqual(first.NodeContent, created.Current.NodeContent) || first.Note != "首版" ||
		first.ChangeKind != ChangeCreate || !first.CreatedAt.Equal(created.Current.CreatedAt) {
		t.Fatalf("revision 1 changed after an edit:\n got %+v\nwant %+v", first, created.Current)
	}

	rescheduled := edited
	rescheduled.LeadDays = nil
	moved, err := fx.store.ReviseNode(ctx, workspace, actor, created.NodeID,
		ReviseNodeRequest{BaseRevision: 2, Content: rescheduled})
	if err != nil {
		t.Fatal(err)
	}
	if moved.Current.ChangeKind != ChangeReschedule {
		t.Fatalf("lead 14 -> unset = %s, want reschedule", moved.Current.ChangeKind)
	}

	_, err = fx.store.ReviseNode(ctx, workspace, actor, created.NodeID,
		ReviseNodeRequest{BaseRevision: 3, Content: rescheduled})
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "revision" {
		t.Fatalf("identical submit = %v, want 400 on revision", err)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision WHERE node_id=$1`, created.NodeID); n != 3 {
		t.Fatalf("revisions = %d after a refused identical submit, want 3", n)
	}
}

func TestConcurrentRevisionsOnOneBaseConflictExactlyOnce(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-race", "actor-a"
	created, err := fx.store.CreateNode(ctx, workspace, actor, nodeRequest("618", "2026-06-18"))
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() {
			content := nodeRequest("618", "2026-06-18").Content
			content.Goal = "目标 " + strconv.Itoa(i)
			_, errs[i] = fx.store.ReviseNode(ctx, workspace, actor, created.NodeID,
				ReviseNodeRequest{BaseRevision: 1, Content: content})
		})
	}
	wg.Wait()

	succeeded, conflicted := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrConflict):
			conflicted++
		default:
			t.Fatalf("unexpected error %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded %d, conflicted %d; want 1 and 1", succeeded, conflicted)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision WHERE node_id=$1`, created.NodeID); n != 2 {
		t.Fatalf("revisions = %d, want 2", n)
	}
	if _, err = fx.store.ConfirmNode(ctx, workspace, actor, created.NodeID, TransitionRequest{BaseRevision: 1}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale confirm = %v, want conflict", err)
	}
}

func TestACancelledNodeRefusesEveryFurtherChange(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-cancel", "actor-a"
	req := nodeRequest("品牌日", "2026-12-01")
	created, err := fx.store.CreateNode(ctx, workspace, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := fx.store.CancelNode(ctx, workspace, actor, created.NodeID,
		TransitionRequest{BaseRevision: 1, Note: "活动取消"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != NodeStatusCancelled || cancelled.Current.ChangeKind != ChangeCancel ||
		cancelled.Current.StatusAfter != NodeStatusCancelled || cancelled.Current.Note != "活动取消" {
		t.Fatalf("cancelled = %+v", cancelled)
	}

	edited := req.Content
	edited.Goal = "换个目标"
	_, reviseErr := fx.store.ReviseNode(ctx, workspace, actor, created.NodeID, ReviseNodeRequest{BaseRevision: 2, Content: edited})
	_, confirmErr := fx.store.ConfirmNode(ctx, workspace, actor, created.NodeID, TransitionRequest{BaseRevision: 2})
	_, cancelErr := fx.store.CancelNode(ctx, workspace, actor, created.NodeID, TransitionRequest{BaseRevision: 2})
	for name, err := range map[string]error{"revise": reviseErr, "confirm": confirmErr, "cancel": cancelErr} {
		var fieldErr FieldError
		if !errors.As(err, &fieldErr) || fieldErr.Field != "status" {
			t.Errorf("%s after cancel = %v, want 400 on status", name, err)
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision WHERE node_id=$1`, created.NodeID); n != 2 {
		t.Fatalf("revisions = %d after refused changes, want 2", n)
	}
}

func importRows(contents ...NodeContent) []ImportRow {
	rows := make([]ImportRow, len(contents))
	for i, content := range contents {
		normalized, err := NormalizeNodeContent(content)
		rows[i] = ImportRow{Content: normalized, Err: err}
	}
	return rows
}

func TestImportCreatesUnconfirmedNodesAndReportsDuplicates(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-import", "actor-a"
	existing, err := fx.store.CreateNode(ctx, workspace, actor, nodeRequest("双十一", "2026-11-11"))
	if err != nil {
		t.Fatal(err)
	}

	same := nodeRequest("  双十一 ", "2026-11-11").Content
	a := nodeRequest("年货节", "2027-01-15").Content
	a.DateCertainty = DateTentative
	a.DateBasis = "去年同期"
	b := nodeRequest("春节", "2027-02-06").Content
	result, err := fx.store.ImportNodes(ctx, workspace, actor, importRows(same, a, b))
	if err != nil {
		t.Fatal(err)
	}
	outcomes := result.Results
	if len(outcomes) != 3 || outcomes[0].Outcome != ImportDuplicate || outcomes[0].NodeID != existing.NodeID ||
		outcomes[1].Outcome != ImportCreated || outcomes[2].Outcome != ImportCreated ||
		outcomes[0].Row != 1 || outcomes[2].Row != 3 {
		t.Fatalf("first import = %+v", outcomes)
	}
	imported, err := fx.store.GetNode(ctx, workspace, actor, outcomes[1].NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if imported.Status != NodeStatusUnconfirmed || imported.Origin != NodeOriginImport ||
		imported.Current.DateCertainty != DateTentative || imported.Current.DateBasis != "去年同期" {
		t.Fatalf("imported node = %+v", imported)
	}

	again, err := fx.store.ImportNodes(ctx, workspace, actor, importRows(same, a, b))
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range again.Results {
		if row.Outcome != ImportDuplicate {
			t.Errorf("second import row %d = %+v, want duplicate", i+1, row)
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node WHERE workspace_id=$1`, workspace); n != 3 {
		t.Fatalf("nodes = %d, want 3", n)
	}

	// A repeat inside one batch is a duplicate of the row created above it; a
	// malformed row is reported and does not take the others down.
	c := nodeRequest("开学季", "2027-09-01").Content
	bad := c
	bad.Kind = "festival"
	batch, err := fx.store.ImportNodes(ctx, workspace, actor, importRows(c, bad, c))
	if err != nil {
		t.Fatal(err)
	}
	rows := batch.Results
	if rows[0].Outcome != ImportCreated || rows[1].Outcome != ImportInvalid || rows[1].Field != "kind" ||
		rows[2].Outcome != ImportDuplicate || rows[2].NodeID != rows[0].NodeID {
		t.Fatalf("batch = %+v", rows)
	}

	// A cancelled node no longer blocks the same row.
	if _, err = fx.store.CancelNode(ctx, workspace, actor, existing.NodeID, TransitionRequest{BaseRevision: 1}); err != nil {
		t.Fatal(err)
	}
	after, err := fx.store.ImportNodes(ctx, workspace, actor, importRows(same))
	if err != nil {
		t.Fatal(err)
	}
	if after.Results[0].Outcome != ImportCreated {
		t.Fatalf("row matching only a cancelled node = %+v, want created", after.Results[0])
	}
}

func TestConfirmChangesTheStatusAndNothingElse(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-confirm", "actor-a"
	content := nodeRequest("行业大会", "2026-12-10").Content
	content.DateCertainty = DateTentative
	result, err := fx.store.ImportNodes(ctx, workspace, actor, importRows(content))
	if err != nil {
		t.Fatal(err)
	}
	nodeID := result.Results[0].NodeID
	before, err := fx.store.GetNode(ctx, workspace, actor, nodeID)
	if err != nil {
		t.Fatal(err)
	}

	confirmed, err := fx.store.ConfirmNode(ctx, workspace, actor, nodeID, TransitionRequest{BaseRevision: 1, Note: "已核实"})
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Status != NodeStatusActive || confirmed.CurrentRevision != 2 ||
		confirmed.Current.ChangeKind != ChangeConfirm || confirmed.Current.StatusAfter != NodeStatusActive {
		t.Fatalf("confirmed = %+v", confirmed)
	}
	if !reflect.DeepEqual(confirmed.Current.NodeContent, before.Current.NodeContent) {
		t.Fatalf("confirm changed content:\n got %+v\nwant %+v", confirmed.Current.NodeContent, before.Current.NodeContent)
	}
	if confirmed.Current.DateCertainty != DateTentative {
		t.Fatal("confirming a node marked its dates as confirmed")
	}
	_, err = fx.store.ConfirmNode(ctx, workspace, actor, nodeID, TransitionRequest{BaseRevision: 2})
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != "status" {
		t.Fatalf("confirm of an active node = %v, want 400 on status", err)
	}
}

// T018 / SC-009: two brands with a node of the same name see only their own,
// and a foreign id is answered exactly as a missing one.
func TestTwoBrandsWithTheSameNodeNameAreIsolated(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	const brandA, brandB, actor = "workspace-brand-a", "workspace-brand-b", "actor-a"
	nodeA, err := fx.store.CreateNode(ctx, brandA, actor, nodeRequest("双十一", "2026-11-11"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.CreateNode(ctx, brandB, actor, nodeRequest("双十一", "2026-11-11")); err != nil {
		t.Fatal(err)
	}
	for _, brand := range []string{brandA, brandB} {
		nodes, listErr := fx.store.ListNodes(ctx, brand, actor, "")
		if listErr != nil {
			t.Fatal(listErr)
		}
		if len(nodes) != 1 || nodes[0].WorkspaceID != brand {
			t.Fatalf("%s lists %+v", brand, nodes)
		}
	}

	content := nodeRequest("双十一", "2026-11-11").Content
	content.Goal = "改"
	calls := map[string]func(id string) error{
		"get": func(id string) error { _, err := fx.store.GetNode(ctx, brandB, actor, id); return err },
		"revise": func(id string) error {
			_, err := fx.store.ReviseNode(ctx, brandB, actor, id, ReviseNodeRequest{BaseRevision: 1, Content: content})
			return err
		},
		"confirm": func(id string) error {
			_, err := fx.store.ConfirmNode(ctx, brandB, actor, id, TransitionRequest{BaseRevision: 1})
			return err
		},
		"cancel": func(id string) error {
			_, err := fx.store.CancelNode(ctx, brandB, actor, id, TransitionRequest{BaseRevision: 1})
			return err
		},
		"history": func(id string) error { _, err := fx.store.ListNodeRevisions(ctx, brandB, actor, id); return err },
	}
	for name, call := range calls {
		foreign, missing := call(nodeA.NodeID), call(diagnostics.NewID())
		if !errors.Is(foreign, ErrNotFound) || !errors.Is(missing, ErrNotFound) || foreign != missing {
			t.Errorf("%s: foreign = %v, missing = %v; want the same not-found", name, foreign, missing)
		}
	}
	unchanged, err := fx.store.GetNode(ctx, brandA, actor, nodeA.NodeID)
	if err != nil || unchanged.CurrentRevision != 1 || unchanged.Status != NodeStatusActive {
		t.Fatalf("brand A's node after brand B's attempts = %+v, %v", unchanged, err)
	}

	// Import dedupe is per brand: A already has 年货节, B does not.
	if _, err = fx.store.CreateNode(ctx, brandA, actor, nodeRequest("年货节", "2027-01-15")); err != nil {
		t.Fatal(err)
	}
	imported, err := fx.store.ImportNodes(ctx, brandB, actor, importRows(nodeRequest("年货节", "2027-01-15").Content))
	if err != nil {
		t.Fatal(err)
	}
	if imported.Results[0].Outcome != ImportCreated {
		t.Fatalf("brand B import judged against brand A: %+v", imported.Results[0])
	}

	// A foreign account and a foreign material are refused the same way a
	// missing one is, and nothing of the node is written.
	seedNodeAccount(t, fx, brandA, "acct-brand-a")
	insertTestSource(t, fx, brandA, actor, "src-brand-a", "inbox")
	nodesBefore := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node WHERE workspace_id=$1`, brandB)
	revisionsBefore := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision WHERE workspace_id=$1`, brandB)
	withAccount := nodeRequest("品牌日", "2026-12-12")
	withAccount.Content.Accounts = []NodeAccount{{AccountID: "acct-brand-a"}}
	withMissingAccount := nodeRequest("品牌日", "2026-12-12")
	withMissingAccount.Content.Accounts = []NodeAccount{{AccountID: "acct-missing"}}
	withSource := nodeRequest("品牌日", "2026-12-12")
	withSource.Content.MaterialSourceIDs = []string{"src-brand-a"}
	withMissingSource := nodeRequest("品牌日", "2026-12-12")
	withMissingSource.Content.MaterialSourceIDs = []string{"src-missing"}
	for name, req := range map[string]CreateNodeRequest{
		"foreign account": withAccount, "missing account": withMissingAccount,
		"foreign material": withSource, "missing material": withMissingSource,
	} {
		if _, err := fx.store.CreateNode(ctx, brandB, actor, req); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s = %v, want not found", name, err)
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node WHERE workspace_id=$1`, brandB); n != nodesBefore {
		t.Fatalf("refused creates left %d nodes, want %d", n, nodesBefore)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision WHERE workspace_id=$1`, brandB); n != revisionsBefore {
		t.Fatalf("refused creates left %d revisions, want %d", n, revisionsBefore)
	}
}

// T019 / SC-002: unset and zero are stored and read back as different answers.
func TestLeadDaysUnsetAndZeroRoundTripApart(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-lead", "actor-a"

	unset := nodeRequest("未设提前量", "2026-12-20")
	unset.Content.LeadDays = nil
	zero := nodeRequest("零提前量", "2026-12-20")
	zero.Content.LeadDays = lead(0)

	createdUnset, err := fx.store.CreateNode(ctx, workspace, actor, unset)
	if err != nil {
		t.Fatal(err)
	}
	createdZero, err := fx.store.CreateNode(ctx, workspace, actor, zero)
	if err != nil {
		t.Fatal(err)
	}
	readUnset, err := fx.store.GetNode(ctx, workspace, actor, createdUnset.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	readZero, err := fx.store.GetNode(ctx, workspace, actor, createdZero.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if readUnset.Current.LeadDays != nil {
		t.Fatalf("unset lead read back as %d", *readUnset.Current.LeadDays)
	}
	if readZero.Current.LeadDays == nil || *readZero.Current.LeadDays != 0 {
		t.Fatalf("zero lead read back as %v", readZero.Current.LeadDays)
	}
	if readUnset.Timing.Phase != PhaseBeforeStartUnknownLead || readUnset.Timing.PreparationStartsOn != nil {
		t.Fatalf("unset lead timing = %+v", readUnset.Timing)
	}
	if readZero.Timing.Phase != PhaseBeforePreparation || readZero.Timing.PreparationStartsOn == nil ||
		*readZero.Timing.PreparationStartsOn != "2026-12-20" {
		t.Fatalf("zero lead timing = %+v", readZero.Timing)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision
		WHERE workspace_id=$1 AND lead_days IS NULL`, workspace); n != 1 {
		t.Fatalf("rows with NULL lead_days = %d, want 1", n)
	}
}

func TestNodeAuditFailureRollsBackTheWrite(t *testing.T) {
	fx := newNodeFixture(t)
	ctx := t.Context()
	auditID := diagnostics.NewID()
	fx.db.InsertNoID(t, "content_operation_audit", testutil.Cols{
		"event_id": auditID, "workspace_id": "workspace-node-failure", "payload": testutil.Raw(`'{}'::jsonb`),
	}, "event_id=$1", auditID)
	// node id, then the audit event id collides with the row above.
	ids := []string{diagnostics.NewID(), auditID, diagnostics.NewID(), diagnostics.NewID()}
	index := 0
	fx.store.NewID = func() string {
		id := ids[index%len(ids)]
		index++
		return id
	}
	if _, err := fx.store.CreateNode(ctx, "workspace-node-failure", "actor-a", nodeRequest("失败", "2026-12-01")); err == nil {
		t.Fatal("create succeeded even though its audit insert failed")
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node WHERE workspace_id='workspace-node-failure'`); n != 0 {
		t.Fatalf("audit failure left %d nodes", n)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision WHERE workspace_id='workspace-node-failure'`); n != 0 {
		t.Fatalf("audit failure left %d revisions", n)
	}
}
