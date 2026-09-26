package topicplanning

import (
	"context"
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
)

// Real PostgreSQL tests for search suggestions (specs/036 PR 2: T039, T040,
// T043 to T045; FR-030 to FR-041, FR-050, FR-051, FR-103; SC-003, SC-008,
// SC-012). They run through scripts/test-go-db.sh --suite topic-planning and
// skip without its database. The document side is the fakeWorks port from
// search_suggestion_test.go; the real work-editor adapter is exercised in
// internal/handler.

// suggestionFixture is the search fixture plus the suggestion migrations,
// found by name: the numbers are provisional until merge.
type suggestionFixture struct {
	topicFixture
	suggestions *SuggestionStore
	works       *fakeWorks
}

func newSuggestionFixture(t *testing.T) suggestionFixture {
	t.Helper()
	fx := newSearchFixture(t)
	_, current, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(current), "..", "..", "..", "migrations")
	matches, err := filepath.Glob(filepath.Join(dir, "*_content_search_suggestion_*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 7 {
		t.Fatalf("found %d search suggestion migrations, want 7", len(matches))
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
	works := &fakeWorks{bodies: map[string]string{}, latest: map[string]string{}}
	return suggestionFixture{topicFixture: fx, suggestions: &SuggestionStore{Store: fx.store, Works: works}, works: works}
}

// document gives the fake a document with one version and that body.
func (fx suggestionFixture) document(workspace, work, artifact, version, body string) {
	fx.works.bodies[workspace+"/"+work+"/"+artifact+"/"+version] = body
	fx.works.latest[workspace+"/"+work+"/"+artifact] = version
}

func (fx suggestionFixture) theme(t *testing.T, workspace string) string {
	t.Helper()
	theme, err := fx.store.CreateSearchTheme(t.Context(), workspace, "actor-a", validTheme())
	if err != nil {
		t.Fatal(err)
	}
	return theme.ThemeID
}

func (fx suggestionFixture) suggestion(t *testing.T, workspace, themeID, proposed string) SuggestionView {
	t.Helper()
	req := validSuggestion()
	req.Target.ThemeID = themeID
	req.Content.ProposedBody = proposed
	view, err := fx.suggestions.CreateSearchSuggestion(t.Context(), workspace, "actor-a", req)
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func suggestionRowJSON(t *testing.T, fx suggestionFixture, suggestionID string, revision int64) string {
	t.Helper()
	var text string
	if err := fx.store.DB.QueryRow(t.Context(), `SELECT row_to_json(r)::text FROM content_search_suggestion_revision r
		WHERE suggestion_id = $1 AND revision = $2`, suggestionID, revision).Scan(&text); err != nil {
		t.Fatalf("read suggestion %s r%d: %v", suggestionID, revision, err)
	}
	return text
}

// tableSnapshot is every row of each table, as JSON in a fixed order: the
// "nothing changed" of SC-008 read byte for byte.
func tableSnapshot(t *testing.T, fx suggestionFixture, tables ...string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	for _, table := range tables {
		var text string
		if err := fx.store.DB.QueryRow(t.Context(), `SELECT coalesce(string_agg(row_to_json(r)::text, E'\n' ORDER BY row_to_json(r)::text), '')
			FROM `+table+` r`).Scan(&text); err != nil {
			t.Fatalf("snapshot %s: %v", table, err)
		}
		snapshot[table] = text
	}
	return snapshot
}

// FR-030 / FR-031 / FR-034 / FR-035: create reads back every field with the
// theme's current revision, a human author and the difference; a revision
// leaves revision 1 byte for byte; a stale base is 409.
func TestSearchSuggestionRevisionsAreAppendOnly(t *testing.T) {
	fx := newSuggestionFixture(t)
	ctx := t.Context()
	const workspace = "ws-sug"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	fx.store.Sources = &fakeSources{byWorkspace: map[string][]string{workspace: {"src-1"}}}
	themeID := fx.theme(t, workspace)

	req := validSuggestion()
	req.Target.ThemeID = themeID
	req.Content.EvidenceSourceIDs = []string{"src-1"}
	created, err := fx.suggestions.CreateSearchSuggestion(ctx, workspace, "actor-a", req)
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 || created.ThemeRevision != 1 || created.AuthorKind != AuthorHuman ||
		created.RecordedBy != "actor-a" || created.State != StateOpen || !created.BaseIsCurrent {
		t.Fatalf("created = %+v", created)
	}
	read, err := fx.suggestions.GetSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID)
	if err != nil || !reflect.DeepEqual(read.SearchSuggestion, created.SearchSuggestion) {
		t.Fatalf("read back %+v, %v", read, err)
	}
	if read.Diff == nil || !reflect.DeepEqual(*read.Diff, DiffLines(scriptBaseBody, req.Content.ProposedBody)) {
		t.Fatalf("diff = %+v", read.Diff)
	}
	first := suggestionRowJSON(t, fx, created.SuggestionID, 1)

	edited := req.Content
	edited.Rationale = "再补一句依据"
	second, err := fx.suggestions.ReviseSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		SuggestionRevisionRequest{BaseRevision: 1, Content: edited})
	if err != nil || second.Revision != 2 || second.Rationale != "再补一句依据" || second.SuggestionTarget != created.SuggestionTarget {
		t.Fatalf("revision 2 = %+v, %v", second, err)
	}
	if got := suggestionRowJSON(t, fx, created.SuggestionID, 1); got != first {
		t.Fatalf("revision 1 changed:\n%s\n%s", first, got)
	}
	_, err = fx.suggestions.ReviseSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		SuggestionRevisionRequest{BaseRevision: 1, Content: edited})
	wantConflict(t, err)
	if n := fx.db.Count(t, `SELECT count(*) FROM content_operation_audit WHERE workspace_id = $1`, workspace); n != 3 {
		t.Fatalf("%d audit rows, want one per write (theme, create, revise)", n)
	}
	if _, err = fx.suggestions.GetSearchSuggestion(ctx, "ws-other", "actor-a", created.SuggestionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another brand reads the suggestion: %v", err)
	}
}

// FR-031 / FR-013: an archived theme, another brand's theme and another
// brand's version are refused by name, before anything is written.
func TestSearchSuggestionReferencesAreThisBrandsOnly(t *testing.T) {
	fx := newSuggestionFixture(t)
	ctx := t.Context()
	const workspace, other = "ws-sug-refs", "ws-sug-refs-other"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	fx.document(other, "work-9", "doc-9", "v9", "别的品牌")
	themeID, foreignTheme := fx.theme(t, workspace), fx.theme(t, other)
	archived := fx.theme(t, workspace)
	if _, err := fx.store.ReviseSearchTheme(ctx, workspace, "actor-a", archived,
		ThemeRevisionRequest{BaseRevision: 1, Voided: true, Content: validTheme()}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		field string
		edit  func(*SuggestionRequest)
	}{
		{"theme_id", func(r *SuggestionRequest) { r.Target.ThemeID = foreignTheme }},
		{"theme_id", func(r *SuggestionRequest) { r.Target.ThemeID = archived }},
		{"base_version_id", func(r *SuggestionRequest) {
			r.Target.ThemeID, r.Target.WorkID, r.Target.ArtifactID, r.Target.BaseVersionID = themeID, "work-9", "doc-9", "v9"
		}},
		{"proposed_body", func(r *SuggestionRequest) { r.Target.ThemeID, r.Content.ProposedBody = themeID, scriptBaseBody }},
	} {
		req := validSuggestion()
		tc.edit(&req)
		_, err := fx.suggestions.CreateSearchSuggestion(ctx, workspace, "actor-a", req)
		wantSearchField(t, err, tc.field)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_revision`); n != 0 {
		t.Fatalf("%d suggestions written by refused requests", n)
	}
}

// T043 / FR-051 / SC-008: abandoning adds one decision and one audit row.
// Every other row of every table this card or its neighbours hold - the
// suggestion's revisions, effects, themes, topic cards, briefs, accounts
// and their revisions, materials - is byte for byte what it was, and the
// works port saw no write. The base being out of date does not matter.
func TestAbandonChangesNothingButTheDecision(t *testing.T) {
	fx := newSuggestionFixture(t)
	ctx := t.Context()
	const workspace = "ws-abandon"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	seedSearchAccount(t, fx.topicFixture, workspace, "acct-xhs", "xiaohongshu")
	seedSearchCardAndBrief(t, fx.topicFixture, workspace, "card-1", "brief-1")
	themeID := fx.theme(t, workspace)
	kept := fx.suggestion(t, workspace, themeID, "另一条建议的正文")
	abandoned := fx.suggestion(t, workspace, themeID, "被放弃的正文")
	fx.works.latest[workspace+"/work-1/doc-1"] = "v4"

	untouched := []string{
		"content_search_suggestion_revision", "content_search_suggestion_effect", "content_search_theme_revision",
		"content_topic_card", "content_brief_revision", "content_account", "content_account_revision", "content_source",
	}
	before := tableSnapshot(t, fx, untouched...)
	decisions := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_decision`)
	audits := fx.db.Count(t, `SELECT count(*) FROM content_operation_audit`)
	fx.works.calls = nil

	view, err := fx.suggestions.DecideSearchSuggestion(ctx, workspace, "actor-b", abandoned.SuggestionID,
		DecisionRequest{Decision: DecisionAbandon, Revision: 1, Note: "同事已经改过"})
	if err != nil {
		t.Fatal(err)
	}
	if view.State != StateAbandoned || view.Decision == nil || view.Decision.DecidedBy != "actor-b" || view.BaseIsCurrent {
		t.Fatalf("abandoned = %+v", view)
	}
	if after := tableSnapshot(t, fx, untouched...); !reflect.DeepEqual(after, before) {
		for _, table := range untouched {
			if after[table] != before[table] {
				t.Errorf("abandoning changed %s", table)
			}
		}
	}
	if got := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_decision`); got != decisions+1 {
		t.Fatalf("%d decisions, want %d", got, decisions+1)
	}
	if got := fx.db.Count(t, `SELECT count(*) FROM content_operation_audit`); got != audits+1 {
		t.Fatalf("%d audit rows, want %d", got, audits+1)
	}
	if fx.works.wrote() || !reflect.DeepEqual(fx.works.calls, []string{"Document"}) {
		t.Fatalf("works port calls while abandoning: %v", fx.works.calls)
	}
	still, err := fx.suggestions.GetSearchSuggestion(ctx, workspace, "actor-a", kept.SuggestionID)
	if err != nil || still.State != StateOpen || still.Decision != nil {
		t.Fatalf("the other suggestion = %+v, %v", still, err)
	}
}

// T044 / FR-050: two abandons of one suggestion at once. Both are held after
// every check has passed, so only the unique index can decide: one wins, the
// other gets the 409 naming suggestion_id, and exactly one decision exists.
func TestConcurrentAbandonsLeaveExactlyOneDecision(t *testing.T) {
	fx := newSuggestionFixture(t)
	const workspace = "ws-abandon-race"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	created := fx.suggestion(t, workspace, fx.theme(t, workspace), "改过的正文")

	var arrived sync.WaitGroup
	arrived.Add(2)
	release := make(chan struct{})
	go func() {
		waited := make(chan struct{})
		go func() { arrived.Wait(); close(waited) }()
		select {
		case <-waited:
		case <-time.After(10 * time.Second):
			t.Error("both abandons never reached the insert together; the index layer was not exercised")
		}
		close(release)
	}()
	ctx := context.WithValue(context.Background(), decisionInsertHook{}, func() {
		arrived.Done()
		<-release
	})
	errs := make([]error, 2)
	var done sync.WaitGroup
	var mu sync.Mutex
	for i := range 2 {
		done.Add(1)
		go func() {
			defer done.Done()
			_, err := fx.suggestions.DecideSearchSuggestion(ctx, workspace, "actor-"+strconv.Itoa(i), created.SuggestionID,
				DecisionRequest{Decision: DecisionAbandon, Revision: 1})
			mu.Lock()
			errs[i] = err
			mu.Unlock()
		}()
	}
	done.Wait()
	wins, conflicts := 0, 0
	for _, err := range errs {
		conflict, isConflict := errors.AsType[SearchConflict](err)
		switch {
		case err == nil:
			wins++
		case isConflict && conflict.Field == "suggestion_id":
			conflicts++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d (%v), want one of each", wins, conflicts, errs)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_decision WHERE suggestion_id = $1`, created.SuggestionID); n != 1 {
		t.Fatalf("%d decisions stored, want 1", n)
	}
}

// T045 / FR-039 / FR-050: after a decision a suggestion takes no revision
// and no second decision; adopt is 400 naming decision in this version.
func TestDecidedSuggestionTakesNoRevisionOrSecondDecision(t *testing.T) {
	fx := newSuggestionFixture(t)
	ctx := t.Context()
	const workspace = "ws-decided"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	created := fx.suggestion(t, workspace, fx.theme(t, workspace), "改过的正文")

	_, err := fx.suggestions.DecideSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		DecisionRequest{Decision: DecisionAdopt, Revision: 1})
	wantSearchField(t, err, "decision")
	_, err = fx.suggestions.DecideSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		DecisionRequest{Decision: DecisionAbandon, Revision: 2})
	if conflict, ok := errors.AsType[SearchConflict](err); !ok || conflict.Field != "revision" {
		t.Fatalf("a stale revision = %v, want 409 revision", err)
	}
	if _, err = fx.suggestions.DecideSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		DecisionRequest{Decision: DecisionAbandon, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	_, err = fx.suggestions.ReviseSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		SuggestionRevisionRequest{BaseRevision: 1, Content: validSuggestion().Content})
	if conflict, ok := errors.AsType[SearchConflict](err); !ok || conflict.Field != "suggestion_id" {
		t.Fatalf("revising a decided suggestion = %v, want 409 suggestion_id", err)
	}
	_, err = fx.suggestions.DecideSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		DecisionRequest{Decision: DecisionAbandon, Revision: 1})
	if conflict, ok := errors.AsType[SearchConflict](err); !ok || conflict.Field != "suggestion_id" {
		t.Fatalf("a second decision = %v, want 409 suggestion_id", err)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_decision`); n != 1 {
		t.Fatalf("%d decisions, want 1", n)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_revision`); n != 1 {
		t.Fatalf("%d suggestion revisions, want 1", n)
	}
}

// T039 / FR-038 / contract §5: the five states, with the adoption states
// built from decision and effect rows inserted directly (PR 2 has no
// adoption path); base_is_current and theme_changed both ways; the list
// filters on the derived state.
func TestSearchSuggestionStatesAreDerivedOnRead(t *testing.T) {
	fx := newSuggestionFixture(t)
	ctx := t.Context()
	const workspace = "ws-states"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	themeID := fx.theme(t, workspace)
	ids := map[SuggestionState]string{}
	for i, state := range SuggestionStates {
		ids[state] = fx.suggestion(t, workspace, themeID, "正文 "+strconv.Itoa(i)).SuggestionID
	}
	decide := func(state SuggestionState, decision string) {
		fx.db.Exec(t, `INSERT INTO content_search_suggestion_decision (workspace_id, decision_id, suggestion_id,
			suggestion_revision, decision, decided_by) VALUES ($1, $2, $3, 1, $4, 'actor-a')`,
			workspace, "dec-"+string(state), ids[state], decision)
	}
	effect := func(state SuggestionState, id, outcome, version, failure string, at time.Time) {
		fx.db.Exec(t, `INSERT INTO content_search_suggestion_effect (workspace_id, effect_id, decision_id, outcome,
			version_id, failure_code, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			workspace, id, "dec-"+string(state), outcome, version, failure, at)
	}
	decide(StateAbandoned, "abandon")
	decide(StateAdopted, "adopt")
	decide(StateAdoptUnrecorded, "adopt")
	decide(StateAdoptFailed, "adopt")
	now := time.Now().UTC()
	effect(StateAdopted, "eff-1", "failed", "", "storage", now)
	effect(StateAdopted, "eff-2", "done", "v4", "", now.Add(time.Second))
	effect(StateAdoptFailed, "eff-3", "failed", "", "base_moved", now)

	for state, id := range ids {
		view, err := fx.suggestions.GetSearchSuggestion(ctx, workspace, "actor-a", id)
		if err != nil || view.State != state {
			t.Fatalf("%s reads as %s (%v)", state, view.State, err)
		}
		if state == StateAdoptFailed && view.FailureCode != FailureBaseMoved {
			t.Errorf("adopt_failed carries %q", view.FailureCode)
		}
		listed, err := fx.suggestions.ListSearchSuggestions(ctx, workspace, "actor-a", SuggestionFilter{State: state})
		if err != nil || len(listed) != 1 || listed[0].SuggestionID != id {
			t.Fatalf("state filter %s = %+v, %v", state, listed, err)
		}
	}
	open, _ := fx.suggestions.GetSearchSuggestion(ctx, workspace, "actor-a", ids[StateOpen])
	if !open.BaseIsCurrent || open.ThemeChanged {
		t.Fatalf("before any change: %+v", open)
	}
	fx.works.latest[workspace+"/work-1/doc-1"] = "v4"
	if _, err := fx.store.ReviseSearchTheme(ctx, workspace, "actor-a", themeID,
		ThemeRevisionRequest{BaseRevision: 1, Content: validTheme()}); err != nil {
		t.Fatal(err)
	}
	moved, _ := fx.suggestions.GetSearchSuggestion(ctx, workspace, "actor-a", ids[StateOpen])
	if moved.BaseIsCurrent || !moved.ThemeChanged {
		t.Fatalf("after a new version and a theme revision: %+v", moved)
	}
}

// T040 / FR-040 / SC-003: three suggestions on one base compare in creation
// order with same_base; a fourth on another base makes it false; one or five
// ids are refused; no table changes.
func TestCompareSearchSuggestionsIsReadOnly(t *testing.T) {
	fx := newSuggestionFixture(t)
	ctx := t.Context()
	const workspace = "ws-compare"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	themeID := fx.theme(t, workspace)
	ids := []string{}
	for i := range 3 {
		ids = append(ids, fx.suggestion(t, workspace, themeID, "第 "+strconv.Itoa(i)+" 条建议").SuggestionID)
	}
	fx.document(workspace, "work-1", "doc-2", "v7", "另一份稿")
	other := validSuggestion()
	other.Target.ThemeID, other.Target.ArtifactID, other.Target.BaseVersionID = themeID, "doc-2", "v7"
	otherView, err := fx.suggestions.CreateSearchSuggestion(ctx, workspace, "actor-a", other)
	if err != nil {
		t.Fatal(err)
	}
	tables := []string{"content_search_suggestion_revision", "content_search_suggestion_decision",
		"content_search_suggestion_effect", "content_search_theme_revision", "content_operation_audit"}
	before := tableSnapshot(t, fx, tables...)

	comparison, err := fx.suggestions.CompareSearchSuggestions(ctx, workspace, "actor-a", []string{ids[2], ids[0], ids[1]})
	if err != nil || !comparison.SameBase || len(comparison.Suggestions) != 3 {
		t.Fatalf("comparison = %+v, %v", comparison, err)
	}
	for i, view := range comparison.Suggestions {
		if view.SuggestionID != ids[i] || view.Diff == nil {
			t.Fatalf("position %d: %s (diff %v), want %s in creation order", i, view.SuggestionID, view.Diff != nil, ids[i])
		}
	}
	comparison, err = fx.suggestions.CompareSearchSuggestions(ctx, workspace, "actor-a", []string{ids[0], otherView.SuggestionID})
	if err != nil || comparison.SameBase {
		t.Fatalf("different bases: %+v, %v", comparison, err)
	}
	for _, bad := range [][]string{{ids[0]}, {ids[0], ids[1], ids[2], otherView.SuggestionID, "x"}} {
		_, err = fx.suggestions.CompareSearchSuggestions(ctx, workspace, "actor-a", bad)
		wantSearchField(t, err, "ids")
	}
	if _, err = fx.suggestions.CompareSearchSuggestions(ctx, "ws-compare-other", "actor-a", ids[:2]); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another brand compares them: %v", err)
	}
	if after := tableSnapshot(t, fx, tables...); !reflect.DeepEqual(after, before) {
		t.Fatal("comparing changed a table")
	}
}

// FR-103 / SC-012: once the workspace deletion has committed, create,
// revise and abandon are refused with ErrNotFound and leave nothing.
func TestSearchSuggestionWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	fx := newSuggestionFixture(t)
	ctx := t.Context()
	const workspace = "ws-sug-fence"
	fx.document(workspace, "work-1", "doc-1", "v3", scriptBaseBody)
	themeID := fx.theme(t, workspace)
	created := fx.suggestion(t, workspace, themeID, "改过的正文")

	fx.store.Guard = &fenceRecordingGuard{denied: true}
	fx.works.calls = nil
	req := validSuggestion()
	req.Target.ThemeID = themeID
	if _, err := fx.suggestions.CreateSearchSuggestion(ctx, workspace, "actor-a", req); !errors.Is(err, ErrNotFound) {
		t.Fatalf("create after the deletion = %v", err)
	}
	if _, err := fx.suggestions.ReviseSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		SuggestionRevisionRequest{BaseRevision: 1, Content: req.Content}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revise after the deletion = %v", err)
	}
	if _, err := fx.suggestions.DecideSearchSuggestion(ctx, workspace, "actor-a", created.SuggestionID,
		DecisionRequest{Decision: DecisionAbandon, Revision: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("abandon after the deletion = %v", err)
	}
	if len(fx.works.calls) != 0 {
		t.Fatalf("the works port was read after the fence refused: %v", fx.works.calls)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_revision WHERE workspace_id = $1`, workspace); n != 1 {
		t.Fatalf("%d suggestion rows, want only the one from before", n)
	}
	if n := fx.db.Count(t, `SELECT count(*) FROM content_search_suggestion_decision WHERE workspace_id = $1`, workspace); n != 0 {
		t.Fatalf("%d decisions after the deletion", n)
	}
}
