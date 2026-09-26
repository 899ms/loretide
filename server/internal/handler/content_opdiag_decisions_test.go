package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/035 PR 3 against the real schema: judgements, suggestions,
// decisions, outcomes, profile proposals and todos (T063, T065 to T074,
// T078, T079; SC-006 to SC-008, SC-014 server half; D14-V05, D14-V08 server
// half). The database cases run in the handler suite
// (scripts/test-go-db.sh --suite handler); the decode and adapter mapping
// cases need none.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §1.3 to
// §1.8, §7

var opdiagPR3Tables = []string{
	"content_opdiag_judgement_revision", "content_opdiag_suggestion_revision", "content_opdiag_decision",
	"content_opdiag_effect", "content_opdiag_profile_proposal_revision", "content_opdiag_todo_revision",
}

// opdiagDecisionWorkspace is PR 1's fixture with a scope-only report
// version 1 on account a1, and teardown for the PR 3 tables and anything a
// test creates in topic-planning and ip-profile.
func opdiagDecisionWorkspace(t *testing.T, slug string) (opdiagFixture, string) {
	t.Helper()
	fx := opdiagWorkspace(t, slug)
	t.Cleanup(func() {
		background := context.Background()
		for _, table := range opdiagPR3Tables {
			_, _ = testPool.Exec(background, `DELETE FROM `+table+` WHERE workspace_id = $1`, fx.wsID)
		}
		_, _ = testPool.Exec(background, `DELETE FROM content_topic_card WHERE workspace_id = $1`, fx.wsID)
		_, _ = testPool.Exec(background, `DELETE FROM content_account_revision WHERE workspace_id = $1`, fx.wsID)
		_, _ = testPool.Exec(background, `DELETE FROM content_feedback_excerpt WHERE workspace_id = $1`, fx.wsID)
	})
	h := feedbackHandler(t)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/",
		opdiagRequest("pr3", opdiagParams("account", []string{fx.a1}, ""))), "report_id")
	return fx, reportID
}

// opdiagCounts is the row count of every operating diagnosis table, the
// topic cards and the account revisions of one workspace.
func opdiagCounts(t *testing.T, wsID string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, table := range append(append([]string{}, opdiagTableNames...), append(opdiagPR3Tables,
		"content_topic_card", "content_account_revision")...) {
		counts[table] = opdiagRows(t, table, wsID)
	}
	return counts
}

// recordingCards wraps the real topic card adapter and records every call;
// fail, when set, answers instead of the real adapter.
type recordingCards struct {
	inner feedbacklearning.TopicCardCreator
	mu    sync.Mutex
	calls []string
	fail  error
}

func (c *recordingCards) CreateOnce(ctx context.Context, workspaceID, actor, key string, draft feedbacklearning.TopicCardDraft) (string, bool, error) {
	c.mu.Lock()
	c.calls = append(c.calls, "CreateOnce "+key)
	fail := c.fail
	c.mu.Unlock()
	if fail != nil {
		return "", false, fail
	}
	return c.inner.CreateOnce(ctx, workspaceID, actor, key, draft)
}

func (c *recordingCards) Exists(ctx context.Context, workspaceID, actor, topicCardID, accountID string) error {
	c.mu.Lock()
	c.calls = append(c.calls, "Exists "+topicCardID)
	c.mu.Unlock()
	return c.inner.Exists(ctx, workspaceID, actor, topicCardID, accountID)
}

// recordingProfiles wraps the real profile writer and records every call.
type recordingProfiles struct {
	inner feedbacklearning.DiagProfileWriter
	calls []string
}

func (p *recordingProfiles) CurrentProfileText(ctx context.Context, workspaceID, accountID string) (feedbacklearning.DiagProfileText, error) {
	p.calls = append(p.calls, "CurrentProfileText")
	return p.inner.CurrentProfileText(ctx, workspaceID, accountID)
}

func (p *recordingProfiles) ApplyProfilePatches(ctx context.Context, workspaceID, actor, accountID, baseRevisionID string,
	patches []feedbacklearning.ProfilePatch) (string, error) {
	p.calls = append(p.calls, "ApplyProfilePatches")
	return p.inner.ApplyProfilePatches(ctx, workspaceID, actor, accountID, baseRevisionID, patches)
}

// recordedStore is h.opdiagStore() with both write adapters recording.
func recordedStore(h *Handler) (*feedbacklearning.DiagnosisStore, *recordingCards, *recordingProfiles) {
	store := h.opdiagStore()
	cards := &recordingCards{inner: store.TopicCards}
	profiles := &recordingProfiles{inner: store.Profiles}
	store.TopicCards, store.Profiles = cards, profiles
	return store, cards, profiles
}

func opdiagSuggest(t *testing.T, store *feedbacklearning.DiagnosisStore, fx opdiagFixture, reportID string,
	kind feedbacklearning.SuggestionTarget, target string) feedbacklearning.OpDiagSuggestion {
	t.Helper()
	suggestion, err := store.RecordSuggestion(t.Context(), fx.wsID, testUserID, reportID, 1, feedbacklearning.SuggestionInput{
		Body: "下周写一篇面料对比", TargetKind: kind, Target: []byte(target),
	})
	if err != nil {
		t.Fatalf("record %s suggestion: %v", kind, err)
	}
	return suggestion
}

func adopt(revision int, mode feedbacklearning.AdoptMode, link string) feedbacklearning.DecisionInput {
	return feedbacklearning.DecisionInput{SuggestionRevision: revision, Decision: feedbacklearning.DecisionAdopt, Mode: mode, LinkTargetID: link}
}

func conflictNaming(t *testing.T, err error, field string) {
	t.Helper()
	if conflict, ok := errors.AsType[feedbacklearning.DecisionConflict](err); ok && conflict.Field == field {
		return
	}
	if conflict, ok := errors.AsType[feedbacklearning.RevisionConflict](err); ok && conflict.Field == field {
		return
	}
	t.Fatalf("err = %v, want a 409 naming %s", err, field)
}

// ---------------------------------------------------------------- no database

// Decode errors on the PR 3 bodies name only the JSON member: an author, a
// recorder or an outcome the server writes is an unknown member, not
// something a client can set.
func TestContentOpDiagDecisionBodiesNameOnlyTheJSONField(t *testing.T) {
	for _, tc := range []struct {
		body, field string
		target      any
	}{
		{`{"kind":"judgement","author_kind":"ai"}`, "author_kind", &feedbacklearning.JudgementInput{}},
		{`{"evidence_refs":"scope"}`, "evidence_refs", &feedbacklearning.JudgementInput{}},
		{`{"base_revision":"1"}`, "base_revision", &opdiagJudgementRevisionBody{}},
		{`{"body":"x","recorded_by":"someone"}`, "recorded_by", &feedbacklearning.SuggestionInput{}},
		{`{"suggestion_revision":"1"}`, "suggestion_revision", &feedbacklearning.DecisionInput{}},
		{`{"decision":"adopt","effect_state":"done"}`, "effect_state", &feedbacklearning.DecisionInput{}},
		{`{"mode":1}`, "mode", &feedbacklearning.RetryInput{}},
		{`{"base_revision_id":1}`, "base_revision_id", &feedbacklearning.ProposalConfirmInput{}},
		{`{"origin_version_no":"1"}`, "origin_version_no", &feedbacklearning.GapTodoInput{}},
		{`{"account_id":"a1"}`, "account_id", &feedbacklearning.GapTodoInput{}},
		{`{"state":1}`, "state", &opdiagTodoRevisionBody{}},
		{`{"reason":"x"}`, "reason", &struct{}{}},
	} {
		err := decodeOpdiagBody(httptest.NewRecorder(), testutil.JSONRequest("POST", "/", tc.body), tc.target)
		fieldErr, ok := errors.AsType[feedbacklearning.FieldError](err)
		if !ok || fieldErr.Field != tc.field {
			t.Errorf("%s: err = %v, want a FieldError naming %q", tc.body, err, tc.field)
		}
	}
}

// The two write adapters: a workspace that is gone - topic-planning's and
// ip-profile's own "not found", which is what their fences answer after a
// deletion committed - is ErrNotFound, the same 404 as every fenced write;
// a failure of theirs is ErrStorage; neither direction is ever the other.
func TestOpdiagWriteAdaptersMapAMissingWorkspaceToNotFound(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  error
		want error
	}{
		{"topic not found", opdiagTopicError(fmt.Errorf("fenced: %w", topicplanning.ErrNotFound)), feedbacklearning.ErrNotFound},
		{"topic storage", opdiagTopicError(errors.New("connection reset")), feedbacklearning.ErrStorage},
		{"topic storage sentinel", opdiagTopicError(topicplanning.ErrStorage), feedbacklearning.ErrStorage},
		{"topic refused", opdiagTopicError(topicplanning.ErrInvalid), feedbacklearning.ErrInvalid},
		{"profile workspace gone", opdiagProfileError(fmt.Errorf("fenced: %w", ipprofile.ErrWorkspaceGone)), feedbacklearning.ErrNotFound},
		{"profile not found", opdiagProfileError(ipprofile.ErrNotFound), feedbacklearning.ErrNotFound},
		{"profile storage", opdiagProfileError(errors.New("connection reset")), feedbacklearning.ErrStorage},
		{"profile refused", opdiagProfileError(ipprofile.ErrProfile), feedbacklearning.ErrInvalid},
		{"profile revision race", opdiagProfileError(ipprofile.ErrRevisionConflict), feedbacklearning.ErrConflict},
	} {
		if !errors.Is(tc.got, tc.want) {
			t.Errorf("%s -> %v, want %v", tc.name, tc.got, tc.want)
		}
		if errors.Is(tc.want, feedbacklearning.ErrNotFound) && errors.Is(tc.got, feedbacklearning.ErrStorage) ||
			errors.Is(tc.want, feedbacklearning.ErrStorage) && errors.Is(tc.got, feedbacklearning.ErrNotFound) {
			t.Errorf("%s crossed not found and storage", tc.name)
		}
	}
	// Unwired adapters fail closed as storage.
	if _, _, err := (opdiagTopicCards{}).CreateOnce(t.Context(), "ws", "a", "k", feedbacklearning.TopicCardDraft{}); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("unwired CreateOnce = %v", err)
	}
	if err := (opdiagTopicCards{}).Exists(t.Context(), "ws", "a", "c", ""); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("unwired Exists = %v", err)
	}
	if _, err := (opdiagProfileWriter{}).CurrentProfileText(t.Context(), "ws", "a"); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("unwired CurrentProfileText = %v", err)
	}
}

// ---------------------------------------------------------------- reject

// T065 / SC-006 / FR-061 / D14-V05: rejecting a suggestion of each target
// kind adds one decision row and one audit entry, and nothing else: no
// other diagnosis row, no topic card, no account revision, and not a single
// call to either write adapter.
func TestContentOpDiagARejectWritesOnlyTheDecision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-reject")
	h := feedbackHandler(t)
	store, cards, profiles := recordedStore(h)
	suggestions := []feedbacklearning.OpDiagSuggestion{
		opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTopicCard, fmt.Sprintf(`{"account_id":%q}`, fx.a1)),
		opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTodo, `{"title":"补录"}`),
		opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetProfileProposal,
			fmt.Sprintf(`{"account_id":%q,"patches":[{"field":"positioning","value":"面料专家"}]}`, fx.a1)),
	}
	before := opdiagCounts(t, fx.wsID)
	for _, suggestion := range suggestions {
		view, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID,
			feedbacklearning.DecisionInput{SuggestionRevision: 1, Decision: feedbacklearning.DecisionReject, Note: "不做"})
		if err != nil {
			t.Fatalf("reject %s: %v", suggestion.TargetKind, err)
		}
		if view.EffectState != feedbacklearning.EffectReadsNone || len(view.Effects) != 0 {
			t.Fatalf("a reject reads %s with %d outcomes", view.EffectState, len(view.Effects))
		}
		if steps := auditSteps(t, fx.wsID, view.DecisionID, "reject-suggestion"); steps != 1 {
			t.Fatalf("%d reject audit entries for %s", steps, view.DecisionID)
		}
	}
	after := opdiagCounts(t, fx.wsID)
	for table, count := range after {
		want := before[table]
		if table == "content_opdiag_decision" {
			want += len(suggestions)
		}
		if count != want {
			t.Errorf("%s: %d rows after rejecting, want %d", table, count, want)
		}
	}
	if len(cards.calls) != 0 || len(profiles.calls) != 0 {
		t.Fatalf("a reject called the write adapters: %v %v", cards.calls, profiles.calls)
	}
}

// ---------------------------------------------------------------- todo and proposal

// failingAudit is an audit sink that refuses every entry.
type failingAudit struct{}

func (failingAudit) AuditTx(context.Context, pgx.Tx, diagnostics.Scope, diagnostics.Event) error {
	return errors.New("audit refused")
}
func (failingAudit) Technical(context.Context, diagnostics.Event) {}

// T066 / FR-064 / FR-069: adopting into a todo is one transaction of three
// rows - the decision, an open todo, a done outcome pointing at it - and
// when the audit entry cannot be written none of the three stays.
func TestContentOpDiagAdoptingIntoATodoIsOneTransaction(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-todo")
	h := feedbackHandler(t)
	store := h.opdiagStore()
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTodo,
		fmt.Sprintf(`{"title":"补录九月指标","account_id":%q}`, fx.a1))

	failing := h.opdiagStore()
	failing.Store.Diagnostics = failingAudit{}
	before := opdiagCounts(t, fx.wsID)
	if _, err := failing.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(1, "", "")); err == nil {
		t.Fatal("an adoption whose audit failed succeeded")
	}
	if after := opdiagCounts(t, fx.wsID); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("a failed audit left rows:\n%v\n%v", before, after)
	}

	view, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(1, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if view.EffectState != feedbacklearning.EffectReadsDone || len(view.Effects) != 1 ||
		view.Effects[0].TargetKind != feedbacklearning.TargetTodo {
		t.Fatalf("adoption = %+v", view)
	}
	todos, err := store.ListTodos(t.Context(), fx.wsID, testUserID, feedbacklearning.TodoFilter{})
	if err != nil || len(todos) != 1 {
		t.Fatalf("todos = %v, %v", todos, err)
	}
	todo := todos[0]
	if todo.TodoID != view.Effects[0].TargetID || todo.State != feedbacklearning.TodoOpen || todo.Title != "补录九月指标" ||
		todo.AccountID != fx.a1 || todo.OriginKind != feedbacklearning.TodoFromSuggestion || todo.OriginDecisionID != view.DecisionID {
		t.Fatalf("todo = %+v", todo)
	}
	after := opdiagCounts(t, fx.wsID)
	for _, table := range []string{"content_opdiag_decision", "content_opdiag_todo_revision", "content_opdiag_effect"} {
		if after[table] != before[table]+1 {
			t.Errorf("%s: %d -> %d, want one more", table, before[table], after[table])
		}
	}
}

// T067 / SC-007 / FR-065: adopting into a profile proposal is one
// transaction of three rows; the proposal is based on the account's current
// profile revision; ip-profile is not written and not asked to write.
func TestContentOpDiagAdoptingIntoAProposalWritesNoProfile(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-proposal-adopt")
	h := feedbackHandler(t)
	store, cards, profiles := recordedStore(h)
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetProfileProposal,
		fmt.Sprintf(`{"account_id":%q,"patches":[{"field":"positioning","value":"面料专家"}]}`, fx.a1))
	before := opdiagCounts(t, fx.wsID)
	view, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(1, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	after := opdiagCounts(t, fx.wsID)
	if after["content_account_revision"] != before["content_account_revision"] {
		t.Fatal("adopting a proposal wrote an account revision")
	}
	for _, table := range []string{"content_opdiag_decision", "content_opdiag_profile_proposal_revision", "content_opdiag_effect"} {
		if after[table] != before[table]+1 {
			t.Errorf("%s: %d -> %d, want one more", table, before[table], after[table])
		}
	}
	if slicesContain(profiles.calls, "ApplyProfilePatches") || len(cards.calls) != 0 {
		t.Fatalf("adopting a proposal called a write: %v %v", profiles.calls, cards.calls)
	}
	proposals, err := store.ListProposals(t.Context(), fx.wsID, testUserID, feedbacklearning.ProposalFilter{AccountID: fx.a1})
	if err != nil || len(proposals) != 1 {
		t.Fatalf("proposals = %v, %v", proposals, err)
	}
	proposal := proposals[0]
	if proposal.ProposalID != view.Effects[0].TargetID || proposal.State != feedbacklearning.ProposalProposed ||
		proposal.BaseRevisionID != fx.revisionID || !proposal.BaseIsCurrent || len(proposal.Items) != 1 ||
		proposal.Items[0].ProposedValue != "面料专家" || proposal.Items[0].CurrentValue != "" {
		t.Fatalf("proposal = %+v", proposal)
	}
}

func slicesContain(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// profileItems is every item of a profile as (value, status) text, a
// blank status read as pending - ip-profile's own reading of it.
func profileItems(profile ipprofile.ExpressionProfile) map[string]string {
	status := func(s ipprofile.FieldStatus) string {
		if s == "" {
			return string(ipprofile.FieldPending)
		}
		return string(s)
	}
	items := map[string]string{}
	for key, field := range profileTextFields(&profile) {
		items[key] = field.Value + "|" + status(field.Status)
	}
	items["primary_channels"] = strings.Join(profile.PrimaryChannels.Values, ",") + "|" + status(profile.PrimaryChannels.Status)
	items["weekly_hours"] = fmt.Sprint(profile.WeeklyHours.Value) + "|" + status(profile.WeeklyHours.Status)
	items["style_samples"] = strings.Join(profile.StyleSamples.Values, ",") + "|" + status(profile.StyleSamples.Status)
	return items
}

// T073 / SC-007 / FR-066 / Q8: confirming while the account's profile is
// still the base writes one ip-profile revision - the patched item's text
// confirmed, the other ten as they were - and records the proposal
// confirmed with it. A proposal whose base has moved answers 409
// base_revision_id and writes nothing; so does a confirmation naming
// another base. Dismissing writes no revision, and is final.
func TestContentOpDiagConfirmingAProposalComparesTheBaseFirst(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-proposal-confirm")
	h := feedbackHandler(t)
	store := h.opdiagStore()
	service := h.contentAccountService()
	proposalFor := func(field, value string) string {
		suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetProfileProposal,
			fmt.Sprintf(`{"account_id":%q,"patches":[{"field":%q,"value":%q}]}`, fx.a1, field, value))
		view, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(1, "", ""))
		if err != nil {
			t.Fatal(err)
		}
		return view.Effects[0].TargetID
	}
	first, second, third := proposalFor("positioning", "面料专家"), proposalFor("audience", "新手"), proposalFor("content_goals", "转化")
	old, err := service.CurrentPersonaRevision(t.Context(), fx.wsID, fx.a1)
	if err != nil {
		t.Fatal(err)
	}
	revisions := func() int { return opdiagRows(t, "content_account_revision", fx.wsID) }
	start := revisions()

	// Another base named in the request: 409, nothing written.
	_, err = store.ConfirmProposal(t.Context(), fx.wsID, testUserID, first, feedbacklearning.ProposalConfirmInput{BaseRevisionID: "rev-other"})
	conflictNaming(t, err, "base_revision_id")
	if revisions() != start {
		t.Fatal("a confirmation naming another base wrote a revision")
	}

	confirmed, err := store.ConfirmProposal(t.Context(), fx.wsID, testUserID, first,
		feedbacklearning.ProposalConfirmInput{BaseRevisionID: fx.revisionID})
	if err != nil {
		t.Fatal(err)
	}
	if revisions() != start+1 || confirmed.State != feedbacklearning.ProposalConfirmed || confirmed.AppliedRevisionID == "" ||
		confirmed.Revision != 2 {
		t.Fatalf("confirm = %+v, %d revisions (was %d)", confirmed, revisions(), start)
	}
	current, err := service.CurrentPersonaRevision(t.Context(), fx.wsID, fx.a1)
	if err != nil || current.RevisionID != confirmed.AppliedRevisionID {
		t.Fatalf("current revision %s, applied %s (%v)", current.RevisionID, confirmed.AppliedRevisionID, err)
	}
	was, now := profileItems(old.Profile), profileItems(current.Profile)
	for key := range was {
		if key == "positioning" {
			if now[key] != "面料专家|confirmed" {
				t.Errorf("positioning = %s, want 面料专家 confirmed", now[key])
			}
			continue
		}
		if now[key] != was[key] {
			t.Errorf("%s changed: %s -> %s", key, was[key], now[key])
		}
	}
	if len(was) != 11 {
		t.Fatalf("compared %d items, want 11", len(was))
	}

	// The second proposal was based on the revision the first replaced.
	_, err = store.ConfirmProposal(t.Context(), fx.wsID, testUserID, second, feedbacklearning.ProposalConfirmInput{BaseRevisionID: fx.revisionID})
	conflictNaming(t, err, "base_revision_id")
	if revisions() != start+1 {
		t.Fatal("a stale confirmation wrote a revision")
	}
	// Confirming the first again: it is no longer proposed.
	_, err = store.ConfirmProposal(t.Context(), fx.wsID, testUserID, first, feedbacklearning.ProposalConfirmInput{BaseRevisionID: fx.revisionID})
	conflictNaming(t, err, "proposal_id")

	dismissed, err := store.DismissProposal(t.Context(), fx.wsID, testUserID, third)
	if err != nil || dismissed.State != feedbacklearning.ProposalDismissed {
		t.Fatalf("dismiss = %+v, %v", dismissed, err)
	}
	_, err = store.DismissProposal(t.Context(), fx.wsID, testUserID, third)
	conflictNaming(t, err, "proposal_id")
	if revisions() != start+1 {
		t.Fatal("dismissing wrote a revision")
	}
}

// ---------------------------------------------------------------- topic cards

// cardsOf counts the workspace's topic cards.
func cardsOf(t *testing.T, wsID string) int {
	t.Helper()
	return opdiagRows(t, "content_topic_card", wsID)
}

// T068 / SC-007 / FR-062: create is decision, then card, then outcome; the
// card is a draft for the suggestion's account with the suggestion as its
// ip_fit, under the suggestion's key; nothing is started.
func TestContentOpDiagAdoptingIntoATopicCardCreatesADraft(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-card-create")
	h := feedbackHandler(t)
	store, cards, _ := recordedStore(h)
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTopicCard, fmt.Sprintf(`{"account_id":%q}`, fx.a1))
	before := cardsOf(t, fx.wsID)
	briefs := opdiagRows(t, "content_brief_revision", fx.wsID)
	view, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(1, feedbacklearning.ModeCreate, ""))
	if err != nil {
		t.Fatal(err)
	}
	if view.EffectState != feedbacklearning.EffectReadsDone || len(view.Effects) != 1 || view.Effects[0].TargetID == "" {
		t.Fatalf("adoption = %+v", view)
	}
	if len(cards.calls) != 1 || cards.calls[0] != "CreateOnce opdiag-suggestion:"+suggestion.SuggestionID {
		t.Fatalf("adapter calls = %v", cards.calls)
	}
	card, err := h.topicPlanningStore().Get(t.Context(), fx.wsID, testUserID, view.Effects[0].TargetID)
	if err != nil {
		t.Fatal(err)
	}
	if card.Status != topicplanning.StatusDraft || card.IPFit != suggestion.Body || card.AccountID == nil || *card.AccountID != fx.a1 ||
		card.StartedBriefRevisionID != nil {
		t.Fatalf("card = %+v", card)
	}
	var key string
	if err = testPool.QueryRow(t.Context(), `SELECT origin_key FROM content_topic_card WHERE topic_card_id=$1`, card.TopicCardID).Scan(&key); err != nil ||
		key != "opdiag-suggestion:"+suggestion.SuggestionID {
		t.Fatalf("origin key = %q, %v", key, err)
	}
	if cardsOf(t, fx.wsID) != before+1 || opdiagRows(t, "content_brief_revision", fx.wsID) != briefs {
		t.Fatal("adopting did more than create one draft card")
	}
	// The decision was committed before the card, and the outcome after.
	var decided, effected time.Time
	if err = testPool.QueryRow(t.Context(), `SELECT d.created_at, e.created_at FROM content_opdiag_decision d
		JOIN content_opdiag_effect e ON e.workspace_id = d.workspace_id AND e.decision_id = d.decision_id
		WHERE d.decision_id=$1`, view.DecisionID).Scan(&decided, &effected); err != nil {
		t.Fatal(err)
	}
	if !decided.Before(effected) || !decided.Before(card.CreatedAt) || effected.Before(card.CreatedAt) {
		t.Fatalf("order: decision %v, card %v, outcome %v", decided, card.CreatedAt, effected)
	}
}

// T069 / SC-008 / FR-063: a card that could not be created leaves the
// decision and a failed outcome with its code; a retry records a second,
// done outcome; retrying a done decision is 409 decision_id.
func TestContentOpDiagAFailedCardIsRecordedAndRetried(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-card-failed")
	h := feedbackHandler(t)
	store, cards, _ := recordedStore(h)
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTopicCard, `{"account_id":""}`)
	cards.fail = feedbacklearning.ErrStorage
	view, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(1, feedbacklearning.ModeCreate, ""))
	if err != nil {
		t.Fatal(err)
	}
	if view.EffectState != feedbacklearning.EffectReadsFailed || len(view.Effects) != 1 ||
		view.Effects[0].FailureCode != feedbacklearning.FailureStorage || view.Effects[0].TargetID != "" {
		t.Fatalf("failed adoption = %+v", view)
	}
	cards.fail = nil
	retried, err := store.RetryDecision(t.Context(), fx.wsID, testUserID, view.DecisionID, feedbacklearning.RetryInput{})
	if err != nil {
		t.Fatal(err)
	}
	if retried.EffectState != feedbacklearning.EffectReadsDone || len(retried.Effects) != 2 ||
		retried.Effects[1].Outcome != feedbacklearning.OutcomeDone || retried.Effects[1].TargetID == "" {
		t.Fatalf("retry = %+v", retried)
	}
	_, err = store.RetryDecision(t.Context(), fx.wsID, testUserID, view.DecisionID, feedbacklearning.RetryInput{})
	conflictNaming(t, err, "decision_id")
	response := roiCall(t, h.RetryContentOpDiagDecision, fx.wsID, "POST", "/", `{}`, "decisionId", view.DecisionID)
	assertROIField(t, response, http.StatusConflict, "decision_id")
}

// lostOutcome adopts a topic card suggestion with the outcome's recording
// made to fail after the card was created: a card, a decision, and no
// outcome. It answers the store, the decision and the card count before.
func lostOutcome(t *testing.T, h *Handler, fx opdiagFixture, reportID string) (*feedbacklearning.DiagnosisStore, feedbacklearning.OpDiagSuggestion, string, int) {
	t.Helper()
	store := h.opdiagStore()
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTopicCard, fmt.Sprintf(`{"account_id":%q}`, fx.a1))
	before := cardsOf(t, fx.wsID)
	store.BeforeEffectRecord = func(context.Context, string) error { return errors.New("the outcome write failed") }
	if _, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID,
		adopt(1, feedbacklearning.ModeCreate, "")); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Fatalf("the adoption with a lost outcome = %v, want ErrStorage", err)
	}
	store.BeforeEffectRecord = nil
	if cardsOf(t, fx.wsID) != before+1 || opdiagRows(t, "content_opdiag_effect", fx.wsID) != 0 {
		t.Fatalf("after the lost outcome: %d cards (was %d), %d outcomes", cardsOf(t, fx.wsID), before,
			opdiagRows(t, "content_opdiag_effect", fx.wsID))
	}
	annotations, err := store.Annotations(t.Context(), fx.wsID, testUserID, reportID, 1)
	if err != nil || len(annotations.Decisions) != 1 || annotations.Decisions[0].EffectState != feedbacklearning.EffectReadsUnrecorded {
		t.Fatalf("annotations after the lost outcome = %+v, %v", annotations.Decisions, err)
	}
	return store, suggestion, annotations.Decisions[0].DecisionID, before
}

// T070 / SC-008 / FR-063a (the Q3 supplement's acceptance case): the card
// was created and the outcome was not recorded - "adopted, outcome not
// recorded". A retry gets the same card back by the suggestion's key: the
// workspace has exactly one more card than before the adoption, and the
// done outcome points at it. A second retry is 409 decision_id.
func TestContentOpDiagALostOutcomeConvergesOnOneCard(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-card-lost")
	h := feedbackHandler(t)
	store, _, decisionID, before := lostOutcome(t, h, fx, reportID)
	var cardID string
	if err := testPool.QueryRow(t.Context(), `SELECT topic_card_id FROM content_topic_card WHERE workspace_id=$1 AND origin_key IS NOT NULL`,
		fx.wsID).Scan(&cardID); err != nil {
		t.Fatal(err)
	}
	retried, err := store.RetryDecision(t.Context(), fx.wsID, testUserID, decisionID, feedbacklearning.RetryInput{Mode: feedbacklearning.ModeCreate})
	if err != nil {
		t.Fatal(err)
	}
	if cardsOf(t, fx.wsID) != before+1 {
		t.Fatalf("%d cards after the retry, want exactly %d", cardsOf(t, fx.wsID), before+1)
	}
	if retried.EffectState != feedbacklearning.EffectReadsDone || len(retried.Effects) != 1 || retried.Effects[0].TargetID != cardID {
		t.Fatalf("retry = %+v, want done pointing at %s", retried, cardID)
	}
	_, err = store.RetryDecision(t.Context(), fx.wsID, testUserID, decisionID, feedbacklearning.RetryInput{})
	conflictNaming(t, err, "decision_id")
}

// T070a / FR-063a: from the same lost-outcome state, two retries at once
// still leave exactly one more card and one done outcome; the second gets
// 409 decision_id (it waits on the per-decision lock and then finds the
// first one's done outcome). A new revision of the same suggestion,
// adopted again, points at the same card.
func TestContentOpDiagConcurrentRetriesMakeOneCard(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-card-race")
	h := feedbackHandler(t)
	store, suggestion, decisionID, before := lostOutcome(t, h, fx, reportID)
	results := make([]feedbacklearning.DecisionView, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for i := range 2 {
		group.Go(func() {
			<-start
			results[i], errs[i] = store.RetryDecision(context.Background(), fx.wsID, testUserID, decisionID, feedbacklearning.RetryInput{})
		})
	}
	close(start)
	group.Wait()
	wins, conflicts := 0, 0
	var cardID string
	for i, err := range errs {
		switch {
		case err == nil:
			wins++
			cardID = results[i].Effects[len(results[i].Effects)-1].TargetID
		case errors.As(err, new(feedbacklearning.DecisionConflict)):
			conflicts++
		default:
			t.Fatalf("a concurrent retry failed: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("%d done and %d conflicts, want 1 and 1", wins, conflicts)
	}
	var done int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_opdiag_effect WHERE decision_id=$1 AND outcome='done'`,
		decisionID).Scan(&done); err != nil || done != 1 {
		t.Fatalf("%d done outcomes (%v), want 1", done, err)
	}
	if cardsOf(t, fx.wsID) != before+1 {
		t.Fatalf("%d cards after two retries, want exactly %d", cardsOf(t, fx.wsID), before+1)
	}

	// A new revision of the same suggestion, adopted again: the same card.
	base := 1
	revised, err := store.ReviseSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, feedbacklearning.SuggestionInput{
		Body: "改成两篇", TargetKind: feedbacklearning.TargetTopicCard, Target: []byte(fmt.Sprintf(`{"account_id":%q}`, fx.a1)),
	}, feedbacklearning.Revision{BaseRevision: &base})
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(revised.Revision, feedbacklearning.ModeCreate, ""))
	if err != nil {
		t.Fatal(err)
	}
	if again.Effects[0].TargetID != cardID || cardsOf(t, fx.wsID) != before+1 {
		t.Fatalf("the new revision's card is %s (want %s); %d cards", again.Effects[0].TargetID, cardID, cardsOf(t, fx.wsID))
	}
}

// T071 / FR-062: link checks the card, read only, before anything is
// written: a card that is not here, or is for another account, answers
// like a missing record and leaves no decision. A card that fits is linked
// with the decision and the outcome in one transaction.
func TestContentOpDiagLinkingACardChecksItFirst(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-card-link")
	other := opdiagWorkspace(t, "opdiag-card-link-other")
	h := feedbackHandler(t)
	store := h.opdiagStore()
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTopicCard, fmt.Sprintf(`{"account_id":%q}`, fx.a1))
	unassigned, err := h.topicPlanningStore().Create(t.Context(), testUserID, topicplanning.TopicCard{WorkspaceID: fx.wsID})
	if err != nil {
		t.Fatal(err)
	}
	reference := roiCall(t, h.GetContentOpDiagAnnotations, fx.wsID, "GET", "/", "", "reportId", "no-such-report", "versionNo", "1")
	for name, card := range map[string]string{
		"missing card": "no-such-card", "another account's card": unassigned.TopicCardID, "another brand's card": other.cardID,
	} {
		response := roiCall(t, h.DecideContentOpDiagSuggestion, fx.wsID, "POST", "/",
			fmt.Sprintf(`{"suggestion_revision":1,"decision":"adopt","mode":"link","link_target_id":%q}`, card),
			"suggestionId", suggestion.SuggestionID)
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing record: %s", name, response.Text())
		}
	}
	if opdiagRows(t, "content_opdiag_decision", fx.wsID) != 0 {
		t.Fatal("a refused link left a decision")
	}
	view, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, adopt(1, feedbacklearning.ModeLink, fx.cardID))
	if err != nil {
		t.Fatal(err)
	}
	if view.EffectState != feedbacklearning.EffectReadsDone || view.Effects[0].TargetID != fx.cardID || view.LinkTargetID != fx.cardID {
		t.Fatalf("link = %+v", view)
	}
}

// T072 / FR-060: one revision, one decision. Two deciders parked right
// before the INSERT - past the read check - leave exactly one decision, and
// the unique index turns the other into 409 suggestion_id. Deciding an
// older revision is 409 suggestion_revision; a voided one is refused.
func TestContentOpDiagOneRevisionTakesOneDecision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-one-decision")
	h := feedbackHandler(t)
	store := h.opdiagStore()
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTodo, `{"title":"补录"}`)

	var arrived sync.WaitGroup
	arrived.Add(2)
	release := make(chan struct{})
	store.BeforeDecisionInsert = func(context.Context, string) {
		arrived.Done()
		<-release
	}
	go func() {
		waited := make(chan struct{})
		go func() { arrived.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-time.After(10 * time.Second):
			t.Error("both deciders never reached the insert together; the index layer was not exercised")
		}
		close(release)
	}()
	errs := make([]error, 2)
	var done sync.WaitGroup
	for i := range 2 {
		done.Go(func() {
			decision := feedbacklearning.DecisionReject
			if i == 1 {
				decision = feedbacklearning.DecisionAdopt
			}
			_, errs[i] = store.DecideSuggestion(context.Background(), fx.wsID, testUserID, suggestion.SuggestionID,
				feedbacklearning.DecisionInput{SuggestionRevision: 1, Decision: decision})
		})
	}
	done.Wait()
	store.BeforeDecisionInsert = nil
	wins, conflicts := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case errors.As(err, new(feedbacklearning.DecisionConflict)):
			conflicts++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 || opdiagRows(t, "content_opdiag_decision", fx.wsID) != 1 {
		t.Fatalf("%d wins, %d conflicts, %d decisions", wins, conflicts, opdiagRows(t, "content_opdiag_decision", fx.wsID))
	}
	// Sequentially, the read check answers the same 409 first.
	_, err := store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID,
		feedbacklearning.DecisionInput{SuggestionRevision: 1, Decision: feedbacklearning.DecisionReject})
	conflictNaming(t, err, "suggestion_id")

	// A new revision: deciding the old one is stale, the new one is free.
	base := 1
	if _, err = store.ReviseSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, feedbacklearning.SuggestionInput{
		Body: "改", TargetKind: feedbacklearning.TargetTodo, Target: []byte(`{"title":"补录"}`),
	}, feedbacklearning.Revision{BaseRevision: &base}); err != nil {
		t.Fatal(err)
	}
	_, err = store.DecideSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID,
		feedbacklearning.DecisionInput{SuggestionRevision: 1, Decision: feedbacklearning.DecisionReject})
	conflictNaming(t, err, "suggestion_revision")
	// A voided revision takes no decision.
	base = 2
	if _, err = store.ReviseSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, feedbacklearning.SuggestionInput{},
		feedbacklearning.Revision{BaseRevision: &base, Voided: true}); err != nil {
		t.Fatal(err)
	}
	response := roiCall(t, h.DecideContentOpDiagSuggestion, fx.wsID, "POST", "/", `{"suggestion_revision":3,"decision":"reject"}`,
		"suggestionId", suggestion.SuggestionID)
	assertROIField(t, response, http.StatusBadRequest, "suggestion_revision")
}

// ---------------------------------------------------------------- revisions and todos

// T063 / FR-054: judgements and suggestions are revisions. A stale base is
// 409 base_revision; a void is a new revision; the earlier revision's row
// is byte for byte what it was.
func TestContentOpDiagJudgementsAndSuggestionsAreRevisions(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-revisions")
	h := feedbackHandler(t)
	judgementID, _ := roiCreated(t, roiCall(t, h.RecordContentOpDiagJudgement, fx.wsID, "POST", "/",
		`{"kind":"judgement","basis":"evidence","evidence_refs":["scope"],"body":"只看范围"}`,
		"reportId", reportID, "versionNo", "1"), "judgement_id")
	first := rowJSON(t, "content_opdiag_judgement_revision", "judgement_id", judgementID, 1)
	assertROIField(t, roiCall(t, h.ReviseContentOpDiagJudgement, fx.wsID, "POST", "/",
		`{"base_revision":2,"kind":"judgement","basis":"qualitative","body":"x"}`, "judgementId", judgementID),
		http.StatusConflict, "base_revision")
	assertROIField(t, roiCall(t, h.ReviseContentOpDiagJudgement, fx.wsID, "POST", "/",
		`{"kind":"judgement","basis":"qualitative","body":"x"}`, "judgementId", judgementID),
		http.StatusBadRequest, "base_revision")
	// Step 7 comes before step 8: a reference that is not in the version is
	// named even when the base is stale too.
	assertROIField(t, roiCall(t, h.ReviseContentOpDiagJudgement, fx.wsID, "POST", "/",
		`{"base_revision":5,"kind":"judgement","basis":"evidence","evidence_refs":["account:x/cadence"],"body":"x"}`,
		"judgementId", judgementID), http.StatusBadRequest, "evidence_refs")
	_, voided := roiCreated(t, roiCall(t, h.ReviseContentOpDiagJudgement, fx.wsID, "POST", "/",
		`{"base_revision":1,"voided":true}`, "judgementId", judgementID), "judgement_id")
	if voided["revision"] != float64(2) || voided["voided"] != true || voided["body"] != "只看范围" {
		t.Fatalf("void = %v", voided)
	}
	if got := rowJSON(t, "content_opdiag_judgement_revision", "judgement_id", judgementID, 1); got != first {
		t.Fatalf("revision 1 changed:\n%s\n%s", first, got)
	}

	// A suggestion citing the voided judgement, or a reference the version
	// does not have, is refused by name.
	assertROIField(t, roiCall(t, h.RecordContentOpDiagSuggestion, fx.wsID, "POST", "/",
		fmt.Sprintf(`{"body":"x","target_kind":"todo","target":{"title":"t"},"judgement_ids":[%q]}`, judgementID),
		"reportId", reportID, "versionNo", "1"), http.StatusBadRequest, "judgement_ids")
	assertROIField(t, roiCall(t, h.RecordContentOpDiagSuggestion, fx.wsID, "POST", "/",
		`{"body":"x","target_kind":"profile_proposal","target":{"account_id":"`+fx.a1+`","patches":[{"field":"persona_prompt","value":"x"}]}}`,
		"reportId", reportID, "versionNo", "1"), http.StatusBadRequest, "target.patches.field")
	suggestionID, _ := roiCreated(t, roiCall(t, h.RecordContentOpDiagSuggestion, fx.wsID, "POST", "/",
		`{"body":"补录","target_kind":"todo","target":{"title":"补录定位"}}`, "reportId", reportID, "versionNo", "1"), "suggestion_id")
	firstSuggestion := rowJSON(t, "content_opdiag_suggestion_revision", "suggestion_id", suggestionID, 1)
	assertROIField(t, roiCall(t, h.ReviseContentOpDiagSuggestion, fx.wsID, "POST", "/",
		`{"base_revision":3,"body":"x","target_kind":"todo","target":{"title":"x"}}`, "suggestionId", suggestionID),
		http.StatusConflict, "base_revision")
	roiCreated(t, roiCall(t, h.ReviseContentOpDiagSuggestion, fx.wsID, "POST", "/",
		`{"base_revision":1,"body":"补录两项","target_kind":"todo","target":{"title":"补录定位与受众"}}`, "suggestionId", suggestionID), "suggestion_id")
	if got := rowJSON(t, "content_opdiag_suggestion_revision", "suggestion_id", suggestionID, 1); got != firstSuggestion {
		t.Fatal("suggestion revision 1 changed")
	}
}

// T074 / FR-032: a gap of the version becomes one todo; adding it again is
// 409 origin_gap_key; a key that is not one of the version's gaps is 400;
// the todo's state is a revision; once voided the gap may be added again.
func TestContentOpDiagAGapBecomesOneTodo(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-gap-todo")
	h := feedbackHandler(t)
	gapKey := "scope/profile_field_pending/profile_field/" + fx.a2 + "/audience"
	// a2 is not in this account report: its gap is not this version's.
	add := func(key string) *testutil.Response {
		return roiCall(t, h.AddContentOpDiagTodo, fx.wsID, "POST", "/",
			fmt.Sprintf(`{"origin_report_id":%q,"origin_version_no":1,"origin_gap_key":%q}`, reportID, key))
	}
	assertROIField(t, add(gapKey), http.StatusBadRequest, "origin_gap_key")
	gapKey = "scope/profile_field_pending/profile_field/" + fx.a1 + "/audience"
	todoID, todo := roiCreated(t, add(gapKey), "todo_id")
	if todo["account_id"] != fx.a1 || todo["origin_kind"] != "data_gap" || todo["title"] != gapKey || todo["state"] != "open" {
		t.Fatalf("todo = %v", todo)
	}
	assertROIField(t, add(gapKey), http.StatusConflict, "origin_gap_key")
	_, revised := roiCreated(t, roiCall(t, h.ReviseContentOpDiagTodo, fx.wsID, "POST", "/",
		`{"base_revision":1,"state":"done","note":"已补"}`, "todoId", todoID), "todo_id")
	if revised["state"] != "done" || revised["revision"] != float64(2) || revised["origin_gap_key"] != gapKey {
		t.Fatalf("revised = %v", revised)
	}
	assertROIField(t, roiCall(t, h.ReviseContentOpDiagTodo, fx.wsID, "POST", "/", `{"base_revision":1,"state":"open"}`,
		"todoId", todoID), http.StatusConflict, "base_revision")
	assertROIField(t, roiCall(t, h.ReviseContentOpDiagTodo, fx.wsID, "POST", "/", `{"base_revision":2,"state":"later"}`,
		"todoId", todoID), http.StatusBadRequest, "state")
	roiCreated(t, roiCall(t, h.ReviseContentOpDiagTodo, fx.wsID, "POST", "/", `{"base_revision":2,"voided":true}`,
		"todoId", todoID), "todo_id")
	roiCreated(t, add(gapKey), "todo_id")
	var list struct {
		Todos []map[string]any `json:"todos"`
	}
	roiCall(t, h.ListContentOpDiagTodos, fx.wsID, "GET", "/", "").Want(http.StatusOK).JSON(&list)
	if len(list.Todos) != 1 || list.Todos[0]["todo_id"] == todoID {
		t.Fatalf("todos = %v, want the new one only", list.Todos)
	}
}

// ---------------------------------------------------------------- fence, deletion, refusals

// FR-069 / FR-084 / SC-012: after a workspace deletion has committed, every
// PR 3 write is refused by the fence with the same ErrNotFound, and none
// leaves a row - including a proposal confirmation, whose ip-profile write
// is fenced by ip-profile itself.
func TestContentOpDiagDecisionWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-pr3-fence")
	h := feedbackHandler(t)
	store := h.opdiagStore()
	ctx := t.Context()
	judgement, err := store.RecordJudgement(ctx, fx.wsID, testUserID, reportID, 1, feedbacklearning.JudgementInput{
		Kind: feedbacklearning.JudgementKindJudgement, Basis: feedbacklearning.BasisQualitative, Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	card := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTopicCard, `{}`)
	proposalSuggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetProfileProposal,
		fmt.Sprintf(`{"account_id":%q,"patches":[{"field":"audience","value":"x"}]}`, fx.a1))
	adopted, err := store.DecideSuggestion(ctx, fx.wsID, testUserID, proposalSuggestion.SuggestionID, adopt(1, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	todo, err := store.AddGapTodo(ctx, fx.wsID, testUserID, feedbacklearning.GapTodoInput{
		OriginReportID: reportID, OriginVersionNo: 1, OriginGapKey: "scope/profile_field_pending/profile_field/" + fx.a1 + "/audience"})
	if err != nil {
		t.Fatal(err)
	}
	before := opdiagCounts(t, fx.wsID)
	if _, err = testPool.Exec(ctx, `DELETE FROM workspace WHERE id::text = $1`, fx.wsID); err != nil {
		t.Fatal(err)
	}
	base := 1
	for name, write := range map[string]func() error{
		"judgement": func() error {
			_, err := store.RecordJudgement(ctx, fx.wsID, testUserID, reportID, 1, feedbacklearning.JudgementInput{
				Kind: feedbacklearning.JudgementKindJudgement, Basis: feedbacklearning.BasisQualitative, Body: "y"})
			return err
		},
		"judgement revision": func() error {
			_, err := store.ReviseJudgement(ctx, fx.wsID, testUserID, judgement.JudgementID, feedbacklearning.JudgementInput{},
				feedbacklearning.Revision{BaseRevision: &base, Voided: true})
			return err
		},
		"suggestion": func() error {
			_, err := store.RecordSuggestion(ctx, fx.wsID, testUserID, reportID, 1, feedbacklearning.SuggestionInput{
				Body: "y", TargetKind: feedbacklearning.TargetTopicCard, Target: []byte(`{}`)})
			return err
		},
		"reject": func() error {
			_, err := store.DecideSuggestion(ctx, fx.wsID, testUserID, card.SuggestionID,
				feedbacklearning.DecisionInput{SuggestionRevision: 1, Decision: feedbacklearning.DecisionReject})
			return err
		},
		"adopt a card": func() error {
			_, err := store.DecideSuggestion(ctx, fx.wsID, testUserID, card.SuggestionID, adopt(1, feedbacklearning.ModeCreate, ""))
			return err
		},
		"confirm": func() error {
			_, err := store.ConfirmProposal(ctx, fx.wsID, testUserID, adopted.Effects[0].TargetID,
				feedbacklearning.ProposalConfirmInput{BaseRevisionID: fx.revisionID})
			return err
		},
		"dismiss": func() error {
			_, err := store.DismissProposal(ctx, fx.wsID, testUserID, adopted.Effects[0].TargetID)
			return err
		},
		"todo revision": func() error {
			state := feedbacklearning.TodoDone
			_, err := store.ReviseTodo(ctx, fx.wsID, testUserID, todo.TodoID, feedbacklearning.TodoRevisionInput{State: &state},
				feedbacklearning.Revision{BaseRevision: &base})
			return err
		},
	} {
		if err := write(); !errors.Is(err, feedbacklearning.ErrNotFound) {
			t.Errorf("%s after the delete committed = %v, want ErrNotFound", name, err)
		}
	}
	if after := opdiagCounts(t, fx.wsID); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("fenced writes left rows:\n%v\n%v", before, after)
	}
}

// FR-085: deleting a workspace removes the six PR 3 tables' rows in it and
// none of a neighbour's.
func TestDeleteWorkspaceRemovesOpDiagDecisionRecords(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	target, targetReport := opdiagDecisionWorkspace(t, "opdiag-pr3-delete-target")
	neighbor, neighborReport := opdiagDecisionWorkspace(t, "opdiag-pr3-delete-neighbor")
	h := feedbackHandler(t)
	for _, pair := range []struct {
		fx       opdiagFixture
		reportID string
	}{{target, targetReport}, {neighbor, neighborReport}} {
		store := h.opdiagStore()
		if _, err := store.RecordJudgement(t.Context(), pair.fx.wsID, testUserID, pair.reportID, 1, feedbacklearning.JudgementInput{
			Kind: feedbacklearning.JudgementKindJudgement, Basis: feedbacklearning.BasisQualitative, Body: "x"}); err != nil {
			t.Fatal(err)
		}
		todo := opdiagSuggest(t, store, pair.fx, pair.reportID, feedbacklearning.TargetTodo, `{"title":"t"}`)
		if _, err := store.DecideSuggestion(t.Context(), pair.fx.wsID, testUserID, todo.SuggestionID, adopt(1, "", "")); err != nil {
			t.Fatal(err)
		}
		proposal := opdiagSuggest(t, store, pair.fx, pair.reportID, feedbacklearning.TargetProfileProposal,
			fmt.Sprintf(`{"account_id":%q,"patches":[{"field":"audience","value":"x"}]}`, pair.fx.a1))
		if _, err := store.DecideSuggestion(t.Context(), pair.fx.wsID, testUserID, proposal.SuggestionID, adopt(1, "", "")); err != nil {
			t.Fatal(err)
		}
	}
	request := newRequest(http.MethodDelete, "/api/workspaces/"+target.wsID, nil)
	request = withURLParam(request, "id", target.wsID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)
	for _, table := range opdiagPR3Tables {
		if rows := opdiagRows(t, table, target.wsID); rows != 0 {
			t.Errorf("%s kept %d rows of the deleted workspace", table, rows)
		}
		if rows := opdiagRows(t, table, neighbor.wsID); rows == 0 {
			t.Errorf("%s lost the neighbour's rows", table)
		}
	}
}

// T078 / contract §7.1: a caller who is not a member is refused on every
// PR 3 endpoint exactly like a missing record, and so is an id that belongs
// to another brand; then steps 7, 8 and 9 answer in that order.
func TestContentOpDiagDecisionRefusalsAnswerLikeMissingRecords(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, reportID := opdiagDecisionWorkspace(t, "opdiag-pr3-refusals")
	other, otherReport := opdiagDecisionWorkspace(t, "opdiag-pr3-refusals-other")
	h := feedbackHandler(t)
	store := h.opdiagStore()
	foreign := opdiagSuggest(t, store, other, otherReport, feedbacklearning.TargetProfileProposal,
		fmt.Sprintf(`{"account_id":%q,"patches":[{"field":"audience","value":"x"}]}`, other.a1))
	foreignDecision, err := store.DecideSuggestion(t.Context(), other.wsID, testUserID, foreign.SuggestionID, adopt(1, "", ""))
	if err != nil {
		t.Fatal(err)
	}
	foreignJudgement, err := store.RecordJudgement(t.Context(), other.wsID, testUserID, otherReport, 1, feedbacklearning.JudgementInput{
		Kind: feedbacklearning.JudgementKindJudgement, Basis: feedbacklearning.BasisQualitative, Body: "x"})
	if err != nil {
		t.Fatal(err)
	}
	foreignTodo, err := store.AddGapTodo(t.Context(), other.wsID, testUserID, feedbacklearning.GapTodoInput{
		OriginReportID: otherReport, OriginVersionNo: 1, OriginGapKey: "scope/profile_field_pending/profile_field/" + other.a1 + "/audience"})
	if err != nil {
		t.Fatal(err)
	}
	reference := roiCall(t, h.GetContentOpDiagAnnotations, fx.wsID, "GET", "/", "", "reportId", "no-such-report", "versionNo", "1")
	reference.Want(http.StatusNotFound)

	outsider := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Opdiag PR3 outsider", "slug": fmt.Sprintf("opdiag-pr3-outsider-%d", time.Now().UnixNano()), "description": "x",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id::text = $1`, outsider)
	})
	type call struct {
		handler http.HandlerFunc
		method  string
		body    string
		params  []string
	}
	calls := map[string]call{
		"annotations":    {h.GetContentOpDiagAnnotations, "GET", "", []string{"reportId", reportID, "versionNo", "1"}},
		"judgement":      {h.RecordContentOpDiagJudgement, "POST", `{}`, []string{"reportId", reportID, "versionNo", "1"}},
		"judgement rev":  {h.ReviseContentOpDiagJudgement, "POST", `{}`, []string{"judgementId", foreignJudgement.JudgementID}},
		"suggestion":     {h.RecordContentOpDiagSuggestion, "POST", `{}`, []string{"reportId", reportID, "versionNo", "1"}},
		"suggestion rev": {h.ReviseContentOpDiagSuggestion, "POST", `{}`, []string{"suggestionId", foreign.SuggestionID}},
		"decide":         {h.DecideContentOpDiagSuggestion, "POST", `{}`, []string{"suggestionId", foreign.SuggestionID}},
		"retry":          {h.RetryContentOpDiagDecision, "POST", `{}`, []string{"decisionId", foreignDecision.DecisionID}},
		"proposals":      {h.ListContentOpDiagProfileProposals, "GET", "", nil},
		"confirm":        {h.ConfirmContentOpDiagProfileProposal, "POST", `{}`, []string{"proposalId", foreignDecision.Effects[0].TargetID}},
		"dismiss":        {h.DismissContentOpDiagProfileProposal, "POST", `{}`, []string{"proposalId", foreignDecision.Effects[0].TargetID}},
		"todos":          {h.ListContentOpDiagTodos, "GET", "", nil},
		"add todo":       {h.AddContentOpDiagTodo, "POST", `{}`, nil},
		"todo revision":  {h.ReviseContentOpDiagTodo, "POST", `{}`, []string{"todoId", foreignTodo.TodoID}},
	}
	for name, c := range calls {
		response := roiCall(t, c.handler, outsider, c.method, "/", c.body, c.params...)
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s as a non-member answered differently from a missing record: %s", name, response.Text())
		}
	}
	// Another brand's ids, from a member of this one.
	for name, c := range map[string]call{
		"foreign judgement":  {h.ReviseContentOpDiagJudgement, "POST", `{"base_revision":1,"voided":true}`, []string{"judgementId", foreignJudgement.JudgementID}},
		"foreign suggestion": {h.DecideContentOpDiagSuggestion, "POST", `{"suggestion_revision":1,"decision":"reject"}`, []string{"suggestionId", foreign.SuggestionID}},
		"foreign decision":   {h.RetryContentOpDiagDecision, "POST", `{}`, []string{"decisionId", foreignDecision.DecisionID}},
		"foreign proposal":   {h.DismissContentOpDiagProfileProposal, "POST", `{}`, []string{"proposalId", foreignDecision.Effects[0].TargetID}},
		"foreign todo":       {h.ReviseContentOpDiagTodo, "POST", `{"base_revision":1,"state":"done"}`, []string{"todoId", foreignTodo.TodoID}},
		"foreign report":     {h.RecordContentOpDiagJudgement, "POST", `{"kind":"judgement","basis":"qualitative","body":"x"}`, []string{"reportId", otherReport, "versionNo", "1"}},
		"foreign gap":        {h.AddContentOpDiagTodo, "POST", fmt.Sprintf(`{"origin_report_id":%q,"origin_version_no":1,"origin_gap_key":"x"}`, otherReport), nil},
		"foreign account": {h.RecordContentOpDiagSuggestion, "POST",
			fmt.Sprintf(`{"body":"x","target_kind":"topic_card","target":{"account_id":%q}}`, other.a1), []string{"reportId", reportID, "versionNo", "1"}},
		"version not a number": {h.GetContentOpDiagAnnotations, "GET", "", []string{"reportId", reportID, "versionNo", "one"}},
	} {
		response := roiCall(t, c.handler, fx.wsID, c.method, "/", c.body, c.params...)
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing record: %s", name, response.Text())
		}
	}
	if rows := opdiagRows(t, "content_opdiag_judgement_revision", other.wsID); rows != 1 {
		t.Fatalf("the other brand has %d judgement rows, want its 1", rows)
	}

	// Steps 7, 8, 9 in order, on a suggestion of this brand.
	suggestion := opdiagSuggest(t, store, fx, reportID, feedbacklearning.TargetTodo, `{"title":"t"}`)
	decide := func(body string) *testutil.Response {
		return roiCall(t, h.DecideContentOpDiagSuggestion, fx.wsID, "POST", "/", body, "suggestionId", suggestion.SuggestionID)
	}
	roiCreated(t, decide(`{"suggestion_revision":1,"decision":"reject"}`), "decision_id")
	// Step 9 alone: already decided.
	assertROIField(t, decide(`{"suggestion_revision":1,"decision":"adopt"}`), http.StatusConflict, "suggestion_id")
	// Step 8 before 9: a stale revision on a decided suggestion.
	base := 1
	if _, err = store.ReviseSuggestion(t.Context(), fx.wsID, testUserID, suggestion.SuggestionID, feedbacklearning.SuggestionInput{
		Body: "y", TargetKind: feedbacklearning.TargetTodo, Target: []byte(`{"title":"t"}`)}, feedbacklearning.Revision{BaseRevision: &base}); err != nil {
		t.Fatal(err)
	}
	assertROIField(t, decide(`{"suggestion_revision":1,"decision":"reject"}`), http.StatusConflict, "suggestion_revision")
	// Steps 4 to 6 before 8: a bad decision value on the stale revision.
	assertROIField(t, decide(`{"suggestion_revision":1,"decision":"maybe"}`), http.StatusBadRequest, "decision")
	assertROIField(t, decide(`{"suggestion_revision":2,"decision":"adopt","mode":"create"}`), http.StatusBadRequest, "mode")
}

// ---------------------------------------------------------------- D14-V08 chain

// T079 / SC-014 / D14-V08 (server half): a topic card, a work from it, a
// publication record of the work, one metric and one excerpt recorded
// through the feedback endpoints, a diagnosis with performance and
// audience feedback that holds that record in its scope and its
// performance records, a suggestion on that version to make a topic card,
// adopted with create - and the topic card list has one more draft.
func TestContentOpDiagServerChainFromCardToAdoptedCard(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx, _ := opdiagDecisionWorkspace(t, "opdiag-chain")
	h := feedbackHandler(t)
	feedbackPost(t, h.RecordContentMetric, fx.wsID, "/api/content-metrics", fmt.Sprintf(`{"publication_record_id":%q,
		"platform":"xiaohongshu","account_id":%q,"metric":"favorite","value":12,"unit":"次",
		"stat_window":"发布后 7 天","sampled_at":"2026-09-17T02:00:00Z","evidence_note":""}`, fx.publicationID, fx.a1)).
		Want(http.StatusCreated)
	feedbackPost(t, h.RecordContentFeedback, fx.wsID, "/api/content-feedback", fmt.Sprintf(`{"publication_record_id":%q,
		"source_type":"comment","redacted_excerpt":"想看面料","interpretation":"","tags":["面料"],
		"occurred_at":"2026-09-11T02:00:00Z"}`, fx.publicationID)).Want(http.StatusCreated)
	params := strings.Replace(opdiagParams("account", []string{fx.a1}, `,"comparison_window":{"start":"2026-08-01","end":"2026-08-31"}`),
		`"dimensions":[]`, `"dimensions":[{"key":"performance","metrics":["favorite"]},{"key":"audience_feedback"}]`, 1)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/", opdiagRequest("chain", params)), "report_id")
	var version struct {
		Result feedbacklearning.DiagnosisResult `json:"result"`
	}
	roiCall(t, h.GetContentOpDiagReportVersion, fx.wsID, "GET", "/", "", "reportId", reportID, "versionNo", "1").
		Want(http.StatusOK).JSON(&version)
	if version.Result.Scope.InputCounts.Publications != 1 || version.Result.Scope.InputCounts.Metrics != 1 ||
		version.Result.Scope.InputCounts.Excerpts != 1 {
		t.Fatalf("scope counts = %+v", version.Result.Scope.InputCounts)
	}
	performance := version.Result.Sections[0].Dimensions[feedbacklearning.DimensionPerformance]
	found := false
	for _, record := range performance.Records {
		found = found || (record.Kind == "publication" && record.ID == fx.publicationID)
	}
	if !found {
		t.Fatalf("performance records %v do not hold %s", performance.Records, fx.publicationID)
	}

	listCards := func() int {
		var list struct {
			TopicCards []map[string]any `json:"topic_cards"`
		}
		roiCall(t, h.ListContentTopics, fx.wsID, "GET", "/", "").Want(http.StatusOK).JSON(&list)
		drafts := 0
		for _, card := range list.TopicCards {
			if card["status"] == "draft" {
				drafts++
			}
		}
		return drafts
	}
	before := listCards()
	suggestionID, _ := roiCreated(t, roiCall(t, h.RecordContentOpDiagSuggestion, fx.wsID, "POST", "/",
		fmt.Sprintf(`{"body":"再写一篇面料对比","target_kind":"topic_card","target":{"account_id":%q},
		"evidence_refs":["account:%s/performance/xiaohongshu/favorite"]}`, fx.a1, fx.a1),
		"reportId", reportID, "versionNo", "1"), "suggestion_id")
	_, adopted := roiCreated(t, roiCall(t, h.DecideContentOpDiagSuggestion, fx.wsID, "POST", "/",
		`{"suggestion_revision":1,"decision":"adopt","mode":"create"}`, "suggestionId", suggestionID), "decision_id")
	if adopted["effect_state"] != "done" {
		t.Fatalf("adoption = %v", adopted)
	}
	if after := listCards(); after != before+1 {
		t.Fatalf("draft cards %d -> %d, want one more", before, after)
	}
}
