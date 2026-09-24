package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
)

// Real PostgreSQL tests for candidates, adoption and impact (specs/033 PR 2:
// T034 to T038, T041, T042) and for concurrent imports of the same row
// (controller decision 1 on PR 2). Run through scripts/test-go-db.sh --suite
// topic-planning; they skip without its database. The four downstream tables
// this isolated schema does not hold (works, reviews, delivery tasks,
// publication records) are checked by the handler suite instead.

type testSourceStatusReader struct{ db Database }

func (r testSourceStatusReader) Status(ctx context.Context, workspaceID, sourceID string) (string, bool, error) {
	var status string
	err := r.db.QueryRow(ctx, `SELECT status FROM content_source WHERE workspace_id=$1 AND source_id=$2`,
		workspaceID, sourceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return status, err == nil, err
}

func newCandidateFixture(t *testing.T) topicFixture {
	t.Helper()
	fx := newNodeFixture(t)
	fx.store.SourceStatuses = testSourceStatusReader{db: fx.store.DB}
	return fx
}

func fieldOf(err error) string {
	var fieldErr FieldError
	if errors.As(err, &fieldErr) {
		return fieldErr.Field
	}
	return ""
}

func candidateFor(t *testing.T, candidates []NodeCandidate, accountID string) NodeCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.AccountID == accountID {
			return candidate
		}
	}
	t.Fatalf("no candidate for account %q in %+v", accountID, candidates)
	return NodeCandidate{}
}

func accountNode(t *testing.T, fx topicFixture, workspace, name, startsOn string, accounts ...NodeAccount) MarketingNode {
	t.Helper()
	req := nodeRequest(name, startsOn)
	req.Content.Accounts = accounts
	node, err := fx.store.CreateNode(t.Context(), workspace, "actor-a", req)
	if err != nil {
		t.Fatal(err)
	}
	return node
}

// rowJSON is a whole row as JSON text, for "not one byte changed" checks.
func rowJSON(t *testing.T, fx topicFixture, table, idColumn, id string) string {
	t.Helper()
	var value string
	fx.db.QueryRow(t, `SELECT row_to_json(x)::text FROM `+table+` x WHERE `+idColumn+` = $1`, id).Scan(&value)
	return value
}

// T034 / SC-004 / FR-018 to FR-022.
func TestSyncMakesOneCandidatePerAccountAndNeverDuplicates(t *testing.T) {
	fx := newCandidateFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-sync", "actor-a"
	seedNodeAccount(t, fx, workspace, "acct-sync-1")
	seedNodeAccount(t, fx, workspace, "acct-sync-2")
	node := accountNode(t, fx, workspace, "双十一", "2026-11-11",
		NodeAccount{AccountID: "acct-sync-1", Role: "主推"}, NodeAccount{AccountID: "acct-sync-2", Role: "补充"})

	first, err := fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("first sync = %d candidates, want 2", len(first))
	}
	one := candidateFor(t, first, "acct-sync-1")
	if one.Status != CandidateOpen || one.Angle != "" || !one.InScope || one.Relation.Role != "主推" {
		t.Fatalf("fresh candidate = %+v", one)
	}
	if _, err = fx.store.EditCandidate(ctx, workspace, actor, node.NodeID, one.CandidateID,
		CandidatePatch{Angle: PatchString{Set: true, Value: "囤货清单"}}); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 3)
	errs[0] = func() error { _, err := fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID); return err }()
	for i := 1; i < 3; i++ {
		wg.Go(func() { _, errs[i] = fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("sync %d = %v", i, err)
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_candidate WHERE node_id=$1`, node.NodeID); n != 2 {
		t.Fatalf("candidates after 4 syncs = %d, want 2", n)
	}
	again, err := fx.store.ListCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if kept := candidateFor(t, again, "acct-sync-1"); kept.Angle != "囤货清单" || kept.CandidateID != one.CandidateID {
		t.Fatalf("re-sync touched the angle a person wrote: %+v", kept)
	}

	// A node with no account gets one brand-level candidate, however often
	// and however concurrently it is synced.
	brand := accountNode(t, fx, workspace, "品牌日", "2026-12-12")
	errs = make([]error, 3)
	for i := range errs {
		wg.Go(func() { _, errs[i] = fx.store.SyncCandidates(ctx, workspace, actor, brand.NodeID) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("brand sync %d = %v", i, err)
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_candidate
		WHERE node_id=$1 AND account_id=''`, brand.NodeID); n != 1 {
		t.Fatalf("brand-level candidates = %d, want 1", n)
	}

	// An account taken off the node keeps its candidate, marked out of scope.
	content := node.Current.NodeContent
	content.Accounts = content.Accounts[:1]
	if _, err = fx.store.ReviseNode(ctx, workspace, actor, node.NodeID,
		ReviseNodeRequest{BaseRevision: 1, Content: content}); err != nil {
		t.Fatal(err)
	}
	after, err := fx.store.ListCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 || candidateFor(t, after, "acct-sync-2").InScope || !candidateFor(t, after, "acct-sync-1").InScope {
		t.Fatalf("after removing an account = %+v", after)
	}

	// Unconfirmed and cancelled nodes have no candidates.
	imported, err := fx.store.ImportNodes(ctx, workspace, actor, importRows(nodeRequest("年货节", "2027-01-15").Content))
	if err != nil {
		t.Fatal(err)
	}
	unconfirmed := imported.Results[0].NodeID
	if _, err = fx.store.CancelNode(ctx, workspace, actor, brand.NodeID, TransitionRequest{BaseRevision: 1}); err != nil {
		t.Fatal(err)
	}
	for name, id := range map[string]string{"unconfirmed": unconfirmed, "cancelled": brand.NodeID} {
		before := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_candidate WHERE node_id=$1`, id)
		if _, err := fx.store.SyncCandidates(ctx, workspace, actor, id); fieldOf(err) != "status" {
			t.Errorf("sync of %s node = %v, want 400 on status", name, err)
		}
		if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_candidate WHERE node_id=$1`, id); n != before {
			t.Errorf("refused sync of %s node wrote candidates", name)
		}
	}
}

// T035 / SC-010 / FR-023 to FR-028.
func TestCandidateReadShowsRelationGapsCollisionsAndDuplicates(t *testing.T) {
	fx := newCandidateFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-read", "actor-a"
	seedNodeAccount(t, fx, workspace, "acct-read-1")
	seedNodeAccount(t, fx, workspace, "acct-read-2")
	profile, err := json.Marshal(ipprofile.ExpressionProfile{
		Audience:       ipprofile.TextField{Value: ""},
		ContentPillars: ipprofile.TextField{Value: "选品、清单", Status: ipprofile.FieldConfirmed},
	})
	if err != nil {
		t.Fatal(err)
	}
	fx.db.Exec(t, `INSERT INTO content_account_revision
		(revision_id, account_id, workspace_id, revision, persona_prompt, profile)
		VALUES ($1,'acct-read-1',$2,1,'',$3)`, diagnostics.NewID(), workspace, profile)
	insertTestSource(t, fx, workspace, actor, "src-read-archived", "archived")
	insertTestSource(t, fx, workspace, actor, "src-read-gone", "inbox")
	insertTestSource(t, fx, workspace, actor, "src-read-ok", "organized")

	req := nodeRequest("双十一", "2026-11-14")
	req.Content.Accounts = []NodeAccount{{AccountID: "acct-read-1", Role: "主推"}}
	req.Content.MaterialSourceIDs = []string{"src-read-archived", "src-read-gone", "src-read-ok"}
	req.Content.DateCertainty = DateTentative
	req.Content.DateBasis = "去年同期"
	node, err := fx.store.CreateNode(ctx, workspace, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	fx.db.Exec(t, `DELETE FROM content_source WHERE source_id='src-read-gone'`)

	// Another active node on the same account, overlapping; one on another
	// account only; a cancelled one on the same account.
	overlap := accountNode(t, fx, workspace, "预热周", "2026-11-12", NodeAccount{AccountID: "acct-read-1"})
	accountNode(t, fx, workspace, "别的号", "2026-11-12", NodeAccount{AccountID: "acct-read-2"})
	gone := accountNode(t, fx, workspace, "取消了", "2026-11-12", NodeAccount{AccountID: "acct-read-1"})
	if _, err = fx.store.CancelNode(ctx, workspace, actor, gone.NodeID, TransitionRequest{BaseRevision: 1}); err != nil {
		t.Fatal(err)
	}

	// Cards: one of this account mentioning the name, one dropped, one of
	// another account, one of this account not mentioning it.
	card := func(account, timing string) TopicCard {
		input := completeCard(workspace)
		input.AccountID = stringPointer(account)
		input.Timing = timing
		created, err := fx.store.Create(ctx, actor, input)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	mention := card("acct-read-1", "去年双十一的复盘")
	dropped := card("acct-read-1", "双十一备选")
	if _, err = fx.store.Act(ctx, workspace, actor, dropped.TopicCardID, ActionRequest{Action: ActionDrop}); err != nil {
		t.Fatal(err)
	}
	card("acct-read-2", "双十一另一个号")
	card("acct-read-1", "日常更新")

	candidates, err := fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	c := candidateFor(t, candidates, "acct-read-1")

	if c.Relation.Goal != req.Content.Goal || c.Relation.Role != "主推" || c.Relation.Account == nil {
		t.Fatalf("relation = %+v", c.Relation)
	}
	if a := c.Relation.Account; a.Audience.Status != ipprofile.FieldPending || a.Audience.Value != "" ||
		a.ContentPillars.Status != ipprofile.FieldConfirmed || a.ContentPillars.Value != "选品、清单" ||
		a.ContentGoals.Status != ipprofile.FieldPending {
		t.Fatalf("account relation = %+v, want values as stored and pending where unfilled", a)
	}
	wantGaps := []MaterialGap{{Kind: GapArchived, SourceID: "src-read-archived"}, {Kind: GapMissing, SourceID: "src-read-gone"}}
	if !reflect.DeepEqual(c.MaterialGaps, wantGaps) {
		t.Fatalf("gaps = %+v, want %+v", c.MaterialGaps, wantGaps)
	}
	if len(c.Collisions) != 1 || c.Collisions[0].NodeID != overlap.NodeID || c.Collisions[0].Name != "预热周" {
		t.Fatalf("collisions = %+v, want only %s", c.Collisions, overlap.NodeID)
	}
	if !reflect.DeepEqual(c.DuplicateRisks, []DuplicateRisk{{TopicCardID: mention.TopicCardID, Reason: DuplicateNameMatch}}) {
		t.Fatalf("duplicate risks = %+v, want only %s", c.DuplicateRisks, mention.TopicCardID)
	}
	if c.Origin != NodeOriginManual || c.DateCertainty != DateTentative || c.DateBasis != "去年同期" {
		t.Fatalf("source fields = %s %s %q", c.Origin, c.DateCertainty, c.DateBasis)
	}
	// 2026-11-11 in Shanghai, 3 days to the start, lead 14: short.
	if c.Timing.Today != "2026-11-11" || c.Timing.DaysUntilStart != 3 || c.Timing.LeadShort == nil || !*c.Timing.LeadShort {
		t.Fatalf("timing = %+v", c.Timing)
	}
	if c.Angle != "" {
		t.Fatalf("a fresh candidate carries an angle %q nobody wrote", c.Angle)
	}

	// A node with no materials reports that, not an empty list.
	bare := accountNode(t, fx, workspace, "无素材", "2027-03-08")
	bareCandidates, err := fx.store.SyncCandidates(ctx, workspace, actor, bare.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bareCandidates[0].MaterialGaps, []MaterialGap{{Kind: GapNone}}) ||
		bareCandidates[0].Relation.Account != nil {
		t.Fatalf("brand-level candidate without materials = %+v", bareCandidates[0])
	}
}

// T036 / SC-005 / FR-030 to FR-033, FR-035.
func TestAdoptCreatesOneDraftCardAndStartsNothing(t *testing.T) {
	fx := newCandidateFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-adopt", "actor-a"
	seedNodeAccount(t, fx, workspace, "acct-adopt-1")
	insertTestSource(t, fx, workspace, actor, "src-adopt-1", "inbox")
	req := nodeRequest("双十一", "2026-11-11")
	req.Content.Accounts = []NodeAccount{{AccountID: "acct-adopt-1"}}
	req.Content.MaterialSourceIDs = []string{"src-adopt-1"}
	node, err := fx.store.CreateNode(ctx, workspace, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	candidateID := candidates[0].CandidateID
	if _, err = fx.store.EditCandidate(ctx, workspace, actor, node.NodeID, candidateID,
		CandidatePatch{Angle: PatchString{Set: true, Value: "预算内的囤货清单"}}); err != nil {
		t.Fatal(err)
	}

	counts := func() [3]int {
		return [3]int{
			fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id=$1`, workspace),
			fx.db.Count(t, `SELECT count(*) FROM content_brief_revision WHERE workspace_id=$1`, workspace),
			fx.db.Count(t, `SELECT count(*) FROM content_start_snapshot WHERE workspace_id=$1`, workspace),
		}
	}
	before := counts()

	adopted, err := fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, candidateID, AdoptRequest{Mode: AdoptCreate})
	if err != nil {
		t.Fatal(err)
	}
	card := adopted.TopicCard
	if card.Status != StatusDraft || card.AccountID == nil || *card.AccountID != "acct-adopt-1" ||
		card.IPFit != "预算内的囤货清单" || card.Timing != AdoptionTiming(node.Current.NodeContent) ||
		!reflect.DeepEqual(card.FitSourceIDs, []string{"src-adopt-1"}) || card.StartedBriefRevisionID != nil ||
		card.AudienceProblemJudgment != "" || card.RecommendedAction != "" || len(card.Channels) != 0 ||
		len(card.EvidenceSourceIDs) != 0 {
		t.Fatalf("adopted card = %+v", card)
	}
	if c := adopted.Candidate; c.Status != CandidateAdopted || c.TopicCardID != card.TopicCardID ||
		c.AdoptedRevision == nil || *c.AdoptedRevision != 1 {
		t.Fatalf("adopted candidate = %+v", c)
	}

	// Once more, then twice at the same time: the same card every time.
	var wg sync.WaitGroup
	results := make([]AdoptResult, 3)
	errs := make([]error, 3)
	results[0], errs[0] = fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, candidateID, AdoptRequest{Mode: AdoptCreate})
	for i := 1; i < 3; i++ {
		wg.Go(func() {
			results[i], errs[i] = fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, candidateID, AdoptRequest{Mode: AdoptCreate})
		})
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil || results[i].TopicCard.TopicCardID != card.TopicCardID {
			t.Fatalf("repeat adoption %d = %s, %v; want %s", i, results[i].TopicCard.TopicCardID, errs[i], card.TopicCardID)
		}
	}
	after := counts()
	if after[0] != before[0]+1 || after[1] != before[1] || after[2] != before[2] {
		t.Fatalf("cards/briefs/snapshots %v -> %v; want exactly one more card and nothing started", before, after)
	}

	// Concurrent first adoptions of a fresh candidate also make one card.
	brand := accountNode(t, fx, workspace, "品牌日", "2026-12-12")
	fresh, err := fx.store.SyncCandidates(ctx, workspace, actor, brand.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range results {
		wg.Go(func() {
			results[i], errs[i] = fx.store.AdoptCandidate(ctx, workspace, actor, brand.NodeID, fresh[0].CandidateID, AdoptRequest{Mode: AdoptCreate})
		})
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil || results[i].TopicCard.TopicCardID != results[0].TopicCard.TopicCardID {
			t.Fatalf("concurrent first adoption %d = %+v, %v", i, results[i].TopicCard.TopicCardID, errs[i])
		}
	}
	if results[0].TopicCard.AccountID != nil {
		t.Fatalf("brand-level adoption made a card for account %v", *results[0].TopicCard.AccountID)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_topic_card WHERE workspace_id=$1`, workspace); n != before[0]+2 {
		t.Fatalf("cards = %d, want %d", n, before[0]+2)
	}
}

// T037 / SC-006 / FR-034, SC-009.
func TestLinkingRecordsTheCardAndLeavesItUntouched(t *testing.T) {
	fx := newCandidateFixture(t)
	ctx := t.Context()
	const workspace, other, actor = "workspace-link", "workspace-link-other", "actor-a"
	seedNodeAccount(t, fx, workspace, "acct-link-1")
	seedNodeAccount(t, fx, workspace, "acct-link-2")
	seedNodeAccount(t, fx, other, "acct-link-other")
	node := accountNode(t, fx, workspace, "双十一", "2026-11-11", NodeAccount{AccountID: "acct-link-1"})
	candidates, err := fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	candidateID := candidates[0].CandidateID
	newCard := func(ws, account string) TopicCard {
		input := completeCard(ws)
		input.AccountID = stringPointer(account)
		created, err := fx.store.Create(ctx, actor, input)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}
	foreign := newCard(other, "acct-link-other")
	otherAccount := newCard(workspace, "acct-link-2")
	mine := newCard(workspace, "acct-link-1")

	candidateRow := rowJSON(t, fx, "content_marketing_node_candidate", "candidate_id", candidateID)
	_, foreignErr := fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, candidateID,
		AdoptRequest{Mode: AdoptLink, TopicCardID: foreign.TopicCardID})
	_, missingErr := fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, candidateID,
		AdoptRequest{Mode: AdoptLink, TopicCardID: diagnostics.NewID()})
	if !errors.Is(foreignErr, ErrNotFound) || foreignErr != missingErr {
		t.Fatalf("foreign card = %v, missing card = %v; want the same not-found", foreignErr, missingErr)
	}
	if _, err = fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, candidateID,
		AdoptRequest{Mode: AdoptLink, TopicCardID: otherAccount.TopicCardID}); fieldOf(err) != "topic_card_id" {
		t.Fatalf("card of another account = %v, want 400 on topic_card_id", err)
	}
	if got := rowJSON(t, fx, "content_marketing_node_candidate", "candidate_id", candidateID); got != candidateRow {
		t.Fatalf("refused links changed the candidate:\n%s\n%s", candidateRow, got)
	}

	cardRow := rowJSON(t, fx, "content_topic_card", "topic_card_id", mine.TopicCardID)
	linked, err := fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, candidateID,
		AdoptRequest{Mode: AdoptLink, TopicCardID: mine.TopicCardID})
	if err != nil {
		t.Fatal(err)
	}
	if linked.Candidate.Status != CandidateAdopted || linked.Candidate.TopicCardID != mine.TopicCardID {
		t.Fatalf("linked candidate = %+v", linked.Candidate)
	}
	if got := rowJSON(t, fx, "content_topic_card", "topic_card_id", mine.TopicCardID); got != cardRow {
		t.Fatalf("linking changed the card:\n%s\n%s", cardRow, got)
	}
}

// T038 / FR-020, FR-036.
func TestEditingACandidateChangesOnlyWhatWasSent(t *testing.T) {
	fx := newCandidateFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-edit-cand", "actor-a"
	node := accountNode(t, fx, workspace, "双十一", "2026-11-11")
	candidates, err := fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	id := candidates[0].CandidateID
	edit := func(patch CandidatePatch) (NodeCandidate, error) {
		return fx.store.EditCandidate(ctx, workspace, actor, node.NodeID, id, patch)
	}
	set := func(value string) PatchString { return PatchString{Set: true, Value: value} }

	c, err := edit(CandidatePatch{Angle: set("角度一")})
	if err != nil || c.Angle != "角度一" {
		t.Fatalf("set angle = %+v, %v", c, err)
	}
	c, err = edit(CandidatePatch{Status: set("dismissed"), DismissReason: set("档期太满")})
	if err != nil || c.Status != CandidateDismissed || c.DismissReason != "档期太满" || c.Angle != "角度一" {
		t.Fatalf("dismiss without angle = %+v, %v; want the angle kept", c, err)
	}
	c, err = edit(CandidatePatch{Status: set("open")})
	if err != nil || c.Status != CandidateOpen {
		t.Fatalf("reopen = %+v, %v", c, err)
	}
	c, err = edit(CandidatePatch{Angle: set("")})
	if err != nil || c.Angle != "" {
		t.Fatalf("explicit empty angle = %+v, %v; want cleared", c, err)
	}

	if _, err = fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, id, AdoptRequest{Mode: AdoptCreate}); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"dismissed", "open"} {
		if _, err = edit(CandidatePatch{Status: set(status)}); fieldOf(err) != "status" {
			t.Errorf("adopted -> %s = %v, want 400 on status", status, err)
		}
	}
	if c, err = edit(CandidatePatch{Angle: set("采用后改角度")}); err != nil || c.Angle != "采用后改角度" || c.Status != CandidateAdopted {
		t.Fatalf("angle after adoption = %+v, %v", c, err)
	}

	// Another brand's candidate id is not found, the same as a random one.
	_, foreign := fx.store.EditCandidate(ctx, "workspace-edit-other", actor, node.NodeID, id, CandidatePatch{Angle: set("x")})
	_, missing := fx.store.EditCandidate(ctx, workspace, actor, node.NodeID, diagnostics.NewID(), CandidatePatch{Angle: set("x")})
	if !errors.Is(foreign, ErrNotFound) || foreign != missing {
		t.Fatalf("foreign = %v, missing = %v", foreign, missing)
	}
}

// T041 and T042 / SC-007, SC-008 / FR-038 to FR-040.
func TestImpactListsAdoptedCardsUntilAPersonDecides(t *testing.T) {
	fx := newCandidateFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-impact", "actor-a"
	seedNodeAccount(t, fx, workspace, "acct-impact-1")
	seedNodeAccount(t, fx, workspace, "acct-impact-2")
	node := accountNode(t, fx, workspace, "双十一", "2026-11-11",
		NodeAccount{AccountID: "acct-impact-1"}, NodeAccount{AccountID: "acct-impact-2"})
	candidates, err := fx.store.SyncCandidates(ctx, workspace, actor, node.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	first := candidateFor(t, candidates, "acct-impact-1")
	second := candidateFor(t, candidates, "acct-impact-2")
	adopted, err := fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, first.CandidateID, AdoptRequest{Mode: AdoptCreate})
	if err != nil {
		t.Fatal(err)
	}
	cardID := adopted.TopicCard.TopicCardID
	// Start the card, so a brief and a snapshot exist to prove untouched.
	if _, err = fx.store.Act(ctx, workspace, actor, cardID, ActionRequest{Action: ActionStart, Brief: completeBrief()}); err != nil {
		t.Fatal(err)
	}
	// The adopted card with its brief and snapshots, as whole rows.
	downstream := func() string {
		var value string
		fx.db.QueryRow(t, `SELECT
			(SELECT COALESCE(json_agg(row_to_json(c)), '[]') FROM content_topic_card c WHERE workspace_id=$1 AND topic_card_id=$2)::text ||
			(SELECT COALESCE(json_agg(row_to_json(b) ORDER BY brief_revision_id), '[]') FROM content_brief_revision b WHERE workspace_id=$1 AND topic_card_id=$2)::text ||
			(SELECT COALESCE(json_agg(row_to_json(s) ORDER BY snapshot_id), '[]') FROM content_start_snapshot s WHERE workspace_id=$1 AND topic_card_id=$2)::text`,
			workspace, cardID).Scan(&value)
		return value
	}
	untouched := downstream()

	revision := int64(1)
	revise := func(change func(*NodeContent)) {
		t.Helper()
		current, err := fx.store.GetNode(ctx, workspace, actor, node.NodeID)
		if err != nil {
			t.Fatal(err)
		}
		content := current.Current.NodeContent
		change(&content)
		moved, err := fx.store.ReviseNode(ctx, workspace, actor, node.NodeID,
			ReviseNodeRequest{BaseRevision: current.CurrentRevision, Content: content})
		if err != nil {
			t.Fatal(err)
		}
		revision = moved.CurrentRevision
	}
	impact := func() []ImpactItem {
		t.Helper()
		items, err := fx.store.ListImpact(ctx, workspace, actor, node.NodeID)
		if err != nil {
			t.Fatal(err)
		}
		return items
	}

	revise(func(c *NodeContent) { c.Goal = "只拉新" })
	if items := impact(); len(items) != 0 {
		t.Fatalf("goal-only edit listed %+v", items)
	}

	revise(func(c *NodeContent) { c.StartsOn, c.EndsOn = "2026-11-18", "2026-11-18" })
	items := impact()
	if len(items) != 1 {
		t.Fatalf("after reschedule = %+v, want 1 item", items)
	}
	item := items[0]
	if item.CandidateID != first.CandidateID || item.TopicCardID != cardID || item.AccountID != "acct-impact-1" ||
		item.CardStatus != string(StatusStarted) || item.AdoptedRevision != 1 || item.CurrentRevision != revision ||
		item.Before.StartsOn != "2026-11-11" || item.After.StartsOn != "2026-11-18" ||
		item.Before.LeadDays == nil || *item.Before.LeadDays != 14 || item.Cancelled {
		t.Fatalf("impact item = %+v", item)
	}

	// A decision takes it off the list; the next reschedule brings it back.
	if _, err = fx.store.DecideImpact(ctx, workspace, actor, node.NodeID, first.CandidateID,
		ImpactDecisionRequest{Decision: ImpactKept, Note: "不改"}); err != nil {
		t.Fatal(err)
	}
	if items := impact(); len(items) != 0 {
		t.Fatalf("after deciding = %+v", items)
	}
	revise(func(c *NodeContent) { c.Goal = "再改目标" })
	if items := impact(); len(items) != 0 {
		t.Fatalf("goal edit after a decision listed %+v", items)
	}
	revise(func(c *NodeContent) { c.LeadDays = nil })
	if items := impact(); len(items) != 1 || items[0].CandidateID != first.CandidateID || items[0].After.LeadDays != nil {
		t.Fatalf("after a second reschedule = %+v, want the item back", items)
	}

	// Adopted after the reschedule: nothing moved since, so not listed.
	if _, err = fx.store.AdoptCandidate(ctx, workspace, actor, node.NodeID, second.CandidateID, AdoptRequest{Mode: AdoptCreate}); err != nil {
		t.Fatal(err)
	}
	if items := impact(); len(items) != 1 || items[0].CandidateID != first.CandidateID {
		t.Fatalf("a candidate adopted after the reschedule is listed: %+v", items)
	}

	if _, err = fx.store.CancelNode(ctx, workspace, actor, node.NodeID, TransitionRequest{BaseRevision: revision, Note: "活动取消"}); err != nil {
		t.Fatal(err)
	}
	items = impact()
	if len(items) != 2 || !items[0].Cancelled || !items[1].Cancelled {
		t.Fatalf("after cancel = %+v, want both adopted cards, cancelled", items)
	}
	if _, err = fx.store.DecideImpact(ctx, workspace, actor, node.NodeID, second.CandidateID,
		ImpactDecisionRequest{Decision: ImpactHandled}); err != nil {
		t.Fatal(err)
	}
	if got := downstream(); got != untouched {
		t.Fatalf("rescheduling, cancelling and deciding changed a card, brief or snapshot:\n%s\n%s", untouched, got)
	}
	kept, err := fx.store.readCandidate(ctx, workspace, actor, node.NodeID, first.CandidateID)
	if err != nil {
		t.Fatal(err)
	}
	if kept.TopicCardID != cardID || kept.AdoptedRevision == nil || *kept.AdoptedRevision != 1 ||
		kept.ImpactDecision != ImpactKept || kept.ImpactDecidedBy != actor {
		t.Fatalf("the adoption record moved: %+v", kept)
	}

	// Only an adopted candidate has an impact to decide.
	third := accountNode(t, fx, workspace, "品牌日", "2026-12-12")
	open, err := fx.store.SyncCandidates(ctx, workspace, actor, third.NodeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.store.DecideImpact(ctx, workspace, actor, third.NodeID, open[0].CandidateID,
		ImpactDecisionRequest{Decision: ImpactKept}); fieldOf(err) != "status" {
		t.Fatalf("decision on an open candidate = %v, want 400 on status", err)
	}
}

// Controller decision 1 on PR 2: two imports of the same row at the same time
// create one node; the other import reports it as a duplicate. Two imports of
// the same two rows in opposite orders do not deadlock.
func TestConcurrentImportsOfTheSameRowCreateOneNode(t *testing.T) {
	fx := newCandidateFixture(t)
	ctx := t.Context()
	const workspace, actor = "workspace-import-race", "actor-a"
	a := nodeRequest("双十一", "2026-11-11").Content
	b := nodeRequest("双十二", "2026-12-12").Content

	for round := range 5 {
		var wg sync.WaitGroup
		results := make([]ImportResult, 2)
		errs := make([]error, 2)
		row := a
		row.Name = a.Name + string(rune('A'+round))
		for i := range results {
			wg.Go(func() { results[i], errs[i] = fx.store.ImportNodes(ctx, workspace, actor, importRows(row)) })
		}
		wg.Wait()
		outcomes := []ImportOutcome{}
		for i := range results {
			if errs[i] != nil {
				t.Fatalf("round %d import %d = %v", round, i, errs[i])
			}
			outcomes = append(outcomes, results[i].Results[0].Outcome)
		}
		slices.Sort(outcomes)
		if !reflect.DeepEqual(outcomes, []ImportOutcome{ImportCreated, ImportDuplicate}) {
			t.Fatalf("round %d outcomes = %v, want one created and one duplicate", round, outcomes)
		}
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node WHERE workspace_id=$1`, workspace); n != 5 {
		t.Fatalf("nodes = %d, want 5", n)
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	orders := [][]ImportRow{importRows(a, b), importRows(b, a)}
	for i := range orders {
		wg.Go(func() { _, errs[i] = fx.store.ImportNodes(ctx, workspace, actor, orders[i]) })
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("opposite-order import %d = %v", i, err)
		}
	}
	for _, name := range []string{"双十一", "双十二"} {
		if n := fx.db.Count(t, `SELECT count(*) FROM content_marketing_node_revision
			WHERE workspace_id=$1 AND name=$2`, workspace, name); n != 1 {
			t.Fatalf("%s imported %d times, want 1", name, n)
		}
	}
}
