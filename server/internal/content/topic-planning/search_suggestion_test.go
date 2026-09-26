package topicplanning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// specs/036 PR 2 without a database: the controlled sets, the field rules,
// strict decoding, state derivation, comparison ids, and the write paths over
// a scripted transaction (T037 to T040, T043 to T045 in their local form;
// FR-030 to FR-041, FR-050, FR-051, FR-103, FR-104; SC-003, SC-008). The same
// behaviour against real PostgreSQL is in search_suggestion_integration_test.go
// and in internal/handler.

// ------------------------------------------------------------ scripted store

// scriptAnswer answers the statements that contain match: with the row the
// function builds from the statement's arguments, with no row when row is
// nil, or with err.
type scriptAnswer struct {
	match string
	row   func(args []any) []any
	err   error
}

// scriptedTx answers from its script, first match wins, and records every
// statement it was asked to run.
type scriptedTx struct {
	pgx.Tx
	answers    []scriptAnswer
	statements []string
	committed  bool
}

func (tx *scriptedTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	tx.statements = append(tx.statements, sql)
	for _, answer := range tx.answers {
		if !strings.Contains(sql, answer.match) {
			continue
		}
		switch {
		case answer.err != nil:
			return scriptedRow{err: answer.err}
		case answer.row == nil:
			return scriptedRow{err: pgx.ErrNoRows}
		default:
			return scriptedRow{values: answer.row(args)}
		}
	}
	return scriptedRow{err: errReadFailed}
}
func (tx *scriptedTx) Rollback(context.Context) error { return nil }
func (tx *scriptedTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

// inserts is every INSERT the transaction ran, by table.
func (tx *scriptedTx) inserts() []string {
	tables := []string{}
	for _, statement := range tx.statements {
		if found := regexp.MustCompile(`INSERT INTO (\w+)`).FindStringSubmatch(statement); found != nil {
			tables = append(tables, found[1])
		}
	}
	return tables
}

type scriptedRow struct {
	values []any
	err    error
}

func (r scriptedRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan of %d values into %d targets", len(r.values), len(dest))
	}
	for i, target := range dest {
		into := reflect.ValueOf(target).Elem()
		into.Set(reflect.ValueOf(r.values[i]).Convert(into.Type()))
	}
	return nil
}

// scriptRows answers a multi-row read.
type scriptRows struct {
	match string
	rows  [][]any
	err   error
}

type scriptedDatabase struct {
	tx      *scriptedTx
	reads   []scriptRows
	begins  int
	queried []string
}

func (d *scriptedDatabase) Begin(context.Context) (pgx.Tx, error) {
	d.begins++
	return d.tx, nil
}
func (d *scriptedDatabase) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return d.tx.QueryRow(ctx, sql, args...)
}
func (d *scriptedDatabase) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	d.queried = append(d.queried, sql)
	for _, read := range d.reads {
		if strings.Contains(sql, read.match) {
			if read.err != nil {
				return nil, read.err
			}
			return &scriptedRowSet{rows: read.rows}, nil
		}
	}
	return &scriptedRowSet{}, nil
}

type scriptedRowSet struct {
	pgx.Rows
	rows [][]any
	at   int
}

func (r *scriptedRowSet) Next() bool { r.at++; return r.at <= len(r.rows) }
func (r *scriptedRowSet) Scan(dest ...any) error {
	return scriptedRow{values: r.rows[r.at-1]}.Scan(dest...)
}
func (r *scriptedRowSet) Close()     {}
func (r *scriptedRowSet) Err() error { return nil }

// fakeWorks is SearchWorks over a map: ws/work/artifact/version -> body, and
// ws/work/artifact -> latest version. Apply is not part of PR 2's port; it is
// here so that a path reaching for a write - by a type assertion, say - is
// counted.
type fakeWorks struct {
	bodies map[string]string
	latest map[string]string
	err    error
	calls  []string
}

func (f *fakeWorks) Document(_ context.Context, workspaceID, _, workID, artifactID string) (SearchDocument, error) {
	f.calls = append(f.calls, "Document")
	if f.err != nil {
		return SearchDocument{}, f.err
	}
	latest, ok := f.latest[workspaceID+"/"+workID+"/"+artifactID]
	if !ok {
		return SearchDocument{}, ErrNotFound
	}
	return SearchDocument{WorkID: workID, ArtifactID: artifactID, LatestVersionID: latest, DraftSaved: true}, nil
}

func (f *fakeWorks) VersionBody(_ context.Context, workspaceID, _, workID, artifactID, versionID string) (string, error) {
	f.calls = append(f.calls, "VersionBody")
	if f.err != nil {
		return "", f.err
	}
	body, ok := f.bodies[workspaceID+"/"+workID+"/"+artifactID+"/"+versionID]
	if !ok {
		return "", ErrNotFound
	}
	return body, nil
}

func (f *fakeWorks) Apply(context.Context, string, string, any) (string, error) {
	f.calls = append(f.calls, "Apply")
	return "", nil
}

func (f *fakeWorks) wrote() bool { return slices.Contains(f.calls, "Apply") }

const scriptBaseBody = "羊绒大衣不建议机洗。\n平铺晾干。"

var scriptNow = time.Date(2026, 9, 25, 8, 0, 0, 0, time.UTC)

func validSuggestion() SuggestionRequest {
	return SuggestionRequest{
		Target: SuggestionTarget{WorkID: "work-1", ArtifactID: "doc-1", BaseVersionID: "v3", ThemeID: "theme-1"},
		Content: SuggestionContent{
			TargetQuestion: "羊绒大衣能机洗吗",
			Aspects:        []SuggestionAspect{AspectTopics, AspectBody, AspectTitle},
			Rationale:      "原稿第一段没有正面回答能不能机洗",
			ProposedBody:   "能机洗吗？不建议，只能用羊毛程序。\n平铺晾干。",
		},
	}
}

// storedSuggestionRow is the current revision the script answers.
func storedSuggestionRow(revision int64) []any {
	req := validSuggestion()
	return []any{"ws-a", "sug-1", revision, req.Target.WorkID, req.Target.ArtifactID, req.Target.BaseVersionID,
		req.Target.ThemeID, int64(2), req.Content.TargetQuestion, []string{"body", "title", "topics"},
		req.Content.Rationale, []byte(`[]`), req.Content.ProposedBody, "human", "actor-0", scriptNow}
}

// scriptState is what the script says is stored.
type scriptState struct {
	themeRevision int64
	themeVoided   bool
	themeMissing  bool
	current       int64 // 0: no such suggestion
	decided       bool
}

func scriptedSuggestionStore(state scriptState, guardDenies bool) (*SuggestionStore, *scriptedDatabase, *fakeWorks, *fakeSources) {
	tx := &scriptedTx{}
	tx.answers = []scriptAnswer{
		{match: "INSERT INTO content_search_suggestion_revision", row: func(args []any) []any {
			return append(slices.Clone(args), scriptNow)
		}},
		{match: "INSERT INTO content_search_suggestion_decision", row: func(args []any) []any {
			return []any{args[1], args[2], args[3], args[4], args[5], args[6], scriptNow}
		}},
		{match: "EXISTS (SELECT 1 FROM content_search_suggestion_decision", row: func([]any) []any {
			return []any{state.decided}
		}},
		{match: "FROM content_search_theme_revision", row: func([]any) []any {
			return []any{state.themeRevision, state.themeVoided}
		}},
	}
	if state.themeMissing {
		tx.answers[3].row = nil
	}
	lock := scriptAnswer{match: "LIMIT 1 FOR"}
	current := scriptAnswer{match: "FROM content_search_suggestion_revision WHERE"}
	if state.current > 0 {
		lock.row = func([]any) []any { return []any{state.current} }
		current.row = func([]any) []any { return storedSuggestionRow(state.current) }
	}
	tx.answers = append(tx.answers, lock, current)
	database := &scriptedDatabase{tx: tx}
	works := &fakeWorks{
		bodies: map[string]string{
			"ws-a/work-1/doc-1/v3": scriptBaseBody,
			"ws-b/work-9/doc-9/v9": "别的品牌的正文",
		},
		latest: map[string]string{"ws-a/work-1/doc-1": "v3", "ws-b/work-9/doc-9": "v9"},
	}
	sources := &fakeSources{byWorkspace: map[string][]string{"ws-a": {"src-own"}, "ws-b": {"src-foreign"}}}
	var guard interface {
		LockForContentDiagnosticWrite(context.Context, pgx.Tx, string) error
	} = allowGuard{}
	if guardDenies {
		guard = denyGuard{}
	}
	return &SuggestionStore{
		Store: &Store{DB: database, Diagnostics: &recordingDiagnostics{}, Guard: guard, Sources: sources, Build: "test"},
		Works: works,
	}, database, works, sources
}

// ------------------------------------------------------------ shape

// T037 / contract §3: each set is exactly this big.
func TestSuggestionControlledSetsHaveExactlyTheirSize(t *testing.T) {
	for name, tc := range map[string]struct{ got, want int }{
		"SuggestionAspects":   {len(SuggestionAspects), 4},
		"AuthorKinds":         {len(AuthorKinds), 1},
		"SuggestionDecisions": {len(SuggestionDecisions), 2},
		"EffectOutcomes":      {len(EffectOutcomes), 2},
		"EffectFailures":      {len(EffectFailures), 5},
		"SuggestionStates":    {len(SuggestionStates), 5},
	} {
		if tc.got != tc.want {
			t.Errorf("%s has %d values, want exactly %d", name, tc.got, tc.want)
		}
	}
	if AuthorKinds[0] != AuthorHuman {
		t.Errorf("the one author kind is %q, want human (D1)", AuthorKinds[0])
	}
}

func suggestionMigration(t *testing.T, suffix string) string {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(current), "..", "..", "..", "migrations", "*_"+suffix+".up.sql"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("migration %s: %v %v", suffix, matches, err)
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func quotedValues(sql, pattern string) []string {
	found := regexp.MustCompile(pattern).FindStringSubmatch(sql)
	if found == nil {
		return nil
	}
	values := []string{}
	for _, quoted := range regexp.MustCompile(`'([^']*)'`).FindAllStringSubmatch(found[1], -1) {
		values = append(values, quoted[1])
	}
	return values
}

func stringsOf[T ~string](values []T) []string {
	out := []string{}
	for _, value := range values {
		out = append(out, string(value))
	}
	return out
}

// The migrations' CHECK lists repeat the Go sets as a backstop; read from the
// migrations, they must be the same sets.
func TestSuggestionMigrationChecksMatchTheGoSets(t *testing.T) {
	revision := suggestionMigration(t, "content_search_suggestion_revision")
	decision := suggestionMigration(t, "content_search_suggestion_decision")
	effect := suggestionMigration(t, "content_search_suggestion_effect")
	for name, tc := range map[string]struct{ checked, want []string }{
		"aspects":      {quotedValues(revision, `(?s)aspects <@ ARRAY\[(.*?)\]`), stringsOf(SuggestionAspects)},
		"author_kind":  {quotedValues(revision, `(?s)author_kind IN \((.*?)\)`), stringsOf(AuthorKinds)},
		"decision":     {quotedValues(decision, `(?s)decision IN \((.*?)\)`), stringsOf(SuggestionDecisions)},
		"outcome":      {quotedValues(effect, `(?s)outcome IN \((.*?)\)`), stringsOf(EffectOutcomes)},
		"failure_code": {quotedValues(effect, `(?s)failure_code IN \((.*?)\)`), append([]string{""}, stringsOf(EffectFailures)...)},
	} {
		if !reflect.DeepEqual(tc.checked, tc.want) {
			t.Errorf("%s: migration checks %v, Go set is %v", name, tc.checked, tc.want)
		}
	}
}

// FR-036 / FR-038: nothing derived and nothing that scores is stored. The
// state, the two flags and the diff are computed on read; there is no column
// for a score, a rank, a density or a keyword count.
func TestSuggestionTablesStoreNothingDerivedOrScored(t *testing.T) {
	column := regexp.MustCompile(`(?m)^    ([a-z_]+)\s`)
	for _, table := range []string{"content_search_suggestion_revision", "content_search_suggestion_decision", "content_search_suggestion_effect"} {
		sql := suggestionMigration(t, table)
		columns := column.FindAllStringSubmatch(sql, -1)
		if len(columns) < 6 {
			t.Fatalf("%s: read %d columns; the parser found nothing to check", table, len(columns))
		}
		for _, found := range columns {
			name := found[1]
			for _, forbidden := range []string{"state", "base_is_current", "theme_changed", "diff", "score", "rank", "density", "keyword_count", "seo"} {
				if name == forbidden || strings.HasPrefix(name, "seo") {
					t.Errorf("%s stores %s", table, name)
				}
			}
		}
	}
}

// T038 / FR-030 to FR-034: each field rule refuses by name.
func TestSuggestionFieldRules(t *testing.T) {
	for _, tc := range []struct {
		field string
		edit  func(*SuggestionRequest)
	}{
		{"work_id", func(r *SuggestionRequest) { r.Target.WorkID = " " }},
		{"artifact_id", func(r *SuggestionRequest) { r.Target.ArtifactID = "" }},
		{"base_version_id", func(r *SuggestionRequest) { r.Target.BaseVersionID = "" }},
		{"theme_id", func(r *SuggestionRequest) { r.Target.ThemeID = "" }},
		{"target_question", func(r *SuggestionRequest) { r.Content.TargetQuestion = "  " }},
		{"target_question", func(r *SuggestionRequest) { r.Content.TargetQuestion = strings.Repeat("问", 501) }},
		{"aspects", func(r *SuggestionRequest) { r.Content.Aspects = nil }},
		{"aspects", func(r *SuggestionRequest) { r.Content.Aspects = []SuggestionAspect{"keywords"} }},
		{"aspects", func(r *SuggestionRequest) { r.Content.Aspects = []SuggestionAspect{AspectBody, AspectBody} }},
		{"rationale", func(r *SuggestionRequest) { r.Content.Rationale = "\n " }},
		{"rationale", func(r *SuggestionRequest) { r.Content.Rationale = strings.Repeat("据", 5001) }},
		{"evidence_source_ids", func(r *SuggestionRequest) {
			for i := range 51 {
				r.Content.EvidenceSourceIDs = append(r.Content.EvidenceSourceIDs, fmt.Sprint("src-", i))
			}
		}},
		{"proposed_body", func(r *SuggestionRequest) { r.Content.ProposedBody = strings.Repeat("字", MaxProposedBodyRunes+1) }},
	} {
		req := validSuggestion()
		tc.edit(&req)
		_, targetErr := NormalizeSuggestionTarget(req.Target)
		_, contentErr := NormalizeSuggestionContent(req.Content)
		wantSearchField(t, errors.Join(targetErr, contentErr), tc.field)
	}
	// The accepting side: limits exactly reached, aspects stored sorted, the
	// body kept byte for byte.
	req := validSuggestion()
	req.Content.TargetQuestion = strings.Repeat("问", 500)
	req.Content.Rationale = strings.Repeat("据", 5000)
	req.Content.ProposedBody = " 首行有空格\r\n末尾换行\n"
	content, err := NormalizeSuggestionContent(req.Content)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(content.Aspects, []SuggestionAspect{AspectBody, AspectTitle, AspectTopics}) {
		t.Errorf("aspects stored as %v, want sorted", content.Aspects)
	}
	if content.ProposedBody != req.Content.ProposedBody {
		t.Errorf("the proposed body was changed: %q", content.ProposedBody)
	}
	if target, err := NormalizeSuggestionTarget(SuggestionTarget{WorkID: " w ", ArtifactID: "a", BaseVersionID: "v", ThemeID: "t"}); err != nil || target.WorkID != "w" {
		t.Errorf("target = %+v, %v", target, err)
	}
}

func suggestionJSON(t *testing.T, extra map[string]any) []byte {
	t.Helper()
	req := validSuggestion()
	body := map[string]any{}
	for _, part := range []any{req.Target, req.Content} {
		raw, _ := json.Marshal(part)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
	}
	for key, value := range extra {
		body[key] = value
	}
	out, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// FR-036 / SC-011: a request carrying a score, a rank or a keyword count is
// refused by that member's name; the server-written fields are accepted and
// dropped.
func TestSuggestionStrictDecoding(t *testing.T) {
	for _, field := range []string{"score", "rank", "density", "keyword_count", "seo_score", "state", "diff"} {
		_, err := DecodeSuggestionCreate(suggestionJSON(t, map[string]any{field: 1}))
		wantSearchField(t, err, field)
	}
	_, err := DecodeSuggestionCreate(suggestionJSON(t, map[string]any{"aspects": "body"}))
	wantSearchField(t, err, "aspects")
	req, err := DecodeSuggestionCreate(suggestionJSON(t, map[string]any{
		"recorded_by": "someone else", "theme_revision": 99, "author_kind": "ai",
	}))
	if err != nil || req.Content.ProposedBody != validSuggestion().Content.ProposedBody {
		t.Fatalf("server-written fields were not dropped: %+v %v", req, err)
	}
	_, err = DecodeSuggestionRevision(suggestionJSON(t, nil))
	wantSearchField(t, err, "base_revision")
	revision, err := DecodeSuggestionRevision(suggestionJSON(t, map[string]any{"base_revision": 2}))
	if err != nil || revision.BaseRevision != 2 || revision.Target.WorkID != "work-1" {
		t.Fatalf("revision = %+v, %v", revision, err)
	}
}

// T045 / FR-050: adopt is refused by name in this version; so is anything
// outside the set, a missing revision and a long note. Abandon passes.
func TestSuggestionDecisionDecoding(t *testing.T) {
	for _, tc := range []struct{ body, field string }{
		{`{"decision":"adopt","revision":1}`, "decision"},
		{`{"decision":"","revision":1}`, "decision"},
		{`{"decision":"accept","revision":1}`, "decision"},
		{`{"decision":"abandon"}`, "revision"},
		{`{"decision":"abandon","revision":0}`, "revision"},
		{`{"decision":"abandon","revision":1,"note":"` + strings.Repeat("说", 2001) + `"}`, "note"},
		{`{"decision":"abandon","revision":1,"score":3}`, "score"},
	} {
		_, err := DecodeSuggestionDecision([]byte(tc.body))
		wantSearchField(t, err, tc.field)
	}
	req, err := DecodeSuggestionDecision([]byte(`{"decision":"abandon","revision":2,"note":"同事已经改过"}`))
	if err != nil || req.Decision != DecisionAbandon || req.Revision != 2 || req.Note != "同事已经改过" {
		t.Fatalf("abandon = %+v, %v", req, err)
	}
	if err := ValidateDecisionRequest(DecisionRequest{Decision: DecisionAdopt, Revision: 1}); err == nil {
		t.Fatal("adopt was accepted in PR 2")
	} else if fieldErr, _ := errors.AsType[FieldError](err); fieldErr.Reason != "not available in this version" {
		t.Fatalf("adopt refused as %+v", fieldErr)
	}
}

// T040 / FR-040: two to four distinct ids; the list filter's state is one
// of the five.
func TestSuggestionCompareIDsAndListFilter(t *testing.T) {
	for _, raw := range []string{"", "a", "a,b,c,d,e", "a,a", "a,,b", ","} {
		_, err := ParseCompareIDs(raw)
		wantSearchField(t, err, "ids")
	}
	for raw, want := range map[string][]string{"a,b": {"a", "b"}, " a , b ,c,d": {"a", "b", "c", "d"}} {
		if got, err := ParseCompareIDs(raw); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%q = %v, %v", raw, got, err)
		}
	}
	_, err := ParseSuggestionFilter("", "", "", "rejected")
	wantSearchField(t, err, "state")
	for _, state := range SuggestionStates {
		if _, err := ParseSuggestionFilter("w", "a", "t", string(state)); err != nil {
			t.Errorf("state %s refused: %v", state, err)
		}
	}
}

// T039 / contract §5: each of the five states, from a decision and effects.
func TestDeriveSuggestionStateCoversTheFiveStates(t *testing.T) {
	adopt := &SuggestionDecisionRecord{DecisionID: "d", Decision: DecisionAdopt}
	failed := SuggestionEffect{Outcome: EffectFailed, FailureCode: FailureStorage}
	for _, tc := range []struct {
		name     string
		decision *SuggestionDecisionRecord
		effects  []SuggestionEffect
		state    SuggestionState
		failure  EffectFailure
	}{
		{"no decision", nil, nil, StateOpen, ""},
		{"abandoned", &SuggestionDecisionRecord{Decision: DecisionAbandon}, nil, StateAbandoned, ""},
		{"adopted", adopt, []SuggestionEffect{failed, {Outcome: EffectDone, VersionID: "v4"}}, StateAdopted, ""},
		{"adopt unrecorded", adopt, nil, StateAdoptUnrecorded, ""},
		{"adopt failed", adopt, []SuggestionEffect{{Outcome: EffectFailed, FailureCode: FailureStorage}, {Outcome: EffectFailed, FailureCode: FailureBaseMoved}}, StateAdoptFailed, FailureBaseMoved},
	} {
		state, failure := DeriveSuggestionState(tc.decision, tc.effects)
		if state != tc.state || failure != tc.failure {
			t.Errorf("%s: %s/%q, want %s/%q", tc.name, state, failure, tc.state, tc.failure)
		}
	}
}

// T039 / FR-038: base_is_current and theme_changed, each both ways.
func TestSuggestionViewDerivesBaseAndThemeFlags(t *testing.T) {
	suggestion := SearchSuggestion{SuggestionTarget: SuggestionTarget{BaseVersionID: "v3"}, ThemeRevision: 2}
	for _, tc := range []struct {
		name          string
		latest        string
		theme         themeHead
		themeFound    bool
		baseIsCurrent bool
		themeChanged  bool
	}{
		{"both unchanged", "v3", themeHead{revision: 2}, true, true, false},
		{"a newer version", "v4", themeHead{revision: 2}, true, false, false},
		{"no version at all", "", themeHead{revision: 2}, true, false, false},
		{"a newer theme revision", "v3", themeHead{revision: 3}, true, true, true},
		{"the theme archived", "v3", themeHead{revision: 2, voided: true}, true, true, true},
		{"the theme gone", "v3", themeHead{}, false, true, true},
	} {
		view := viewSuggestion(suggestion, nil, nil, tc.theme, tc.themeFound, SearchDocument{LatestVersionID: tc.latest})
		if view.BaseIsCurrent != tc.baseIsCurrent || view.ThemeChanged != tc.themeChanged {
			t.Errorf("%s: base_is_current %v theme_changed %v", tc.name, view.BaseIsCurrent, view.ThemeChanged)
		}
		if view.State != StateOpen || view.Effects == nil || view.Decision != nil {
			t.Errorf("%s: %+v", tc.name, view)
		}
	}
}

// ------------------------------------------------------------ writes

// FR-030 / FR-031 / FR-034, the accepting side: a suggestion on this brand's
// current theme and version, with a changed body, is written - one row, one
// audit, committed - with the theme's current revision, a human author, and
// its difference; the adapter was only read.
func TestCreateSuggestionPassesItsChecksAndWritesOneRow(t *testing.T) {
	store, database, works, _ := scriptedSuggestionStore(scriptState{themeRevision: 2}, false)
	req := validSuggestion()
	req.Content.EvidenceSourceIDs = []string{"src-own"}
	view, err := store.CreateSearchSuggestion(t.Context(), "ws-a", "actor-a", req)
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != 1 || view.ThemeRevision != 2 || view.AuthorKind != AuthorHuman || view.RecordedBy != "actor-a" ||
		view.State != StateOpen || !view.BaseIsCurrent || view.ThemeChanged || view.Diff == nil || view.Decision != nil {
		t.Fatalf("created = %+v", view)
	}
	if !reflect.DeepEqual(view.Aspects, []SuggestionAspect{AspectBody, AspectTitle, AspectTopics}) {
		t.Errorf("aspects = %v", view.Aspects)
	}
	if want := DiffLines(scriptBaseBody, req.Content.ProposedBody); !reflect.DeepEqual(*view.Diff, want) {
		t.Errorf("diff = %+v, want %+v", *view.Diff, want)
	}
	if inserts := database.tx.inserts(); !reflect.DeepEqual(inserts, []string{"content_search_suggestion_revision"}) || !database.tx.committed {
		t.Fatalf("inserts %v, committed %v", inserts, database.tx.committed)
	}
	if works.wrote() {
		t.Fatal("creating a suggestion wrote through the works adapter")
	}
}

// FR-013 / FR-030 / FR-031 / FR-034, the refusing side: an archived or
// missing theme, a body equal to the base, another brand's version or
// material - each named, each before any row is written; another brand's id
// is refused exactly like a missing one.
func TestCreateSuggestionReferenceRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state scriptState
		edit  func(*SuggestionRequest)
		field string
	}{
		{"archived theme", scriptState{themeRevision: 3, themeVoided: true}, func(*SuggestionRequest) {}, "theme_id"},
		{"missing theme", scriptState{themeMissing: true}, func(*SuggestionRequest) {}, "theme_id"},
		{"unchanged body", scriptState{themeRevision: 2}, func(r *SuggestionRequest) { r.Content.ProposedBody = scriptBaseBody }, "proposed_body"},
	} {
		store, database, _, _ := scriptedSuggestionStore(tc.state, false)
		req := validSuggestion()
		tc.edit(&req)
		_, err := store.CreateSearchSuggestion(t.Context(), "ws-a", "actor-a", req)
		wantSearchField(t, err, tc.field)
		if inserts := database.tx.inserts(); len(inserts) != 0 || database.tx.committed {
			t.Fatalf("%s: inserts %v committed %v", tc.name, inserts, database.tx.committed)
		}
	}
	for _, tc := range []struct {
		field            string
		foreign, missing func(*SuggestionRequest)
	}{
		{"base_version_id",
			func(r *SuggestionRequest) {
				r.Target.WorkID, r.Target.ArtifactID, r.Target.BaseVersionID = "work-9", "doc-9", "v9"
			},
			func(r *SuggestionRequest) { r.Target.BaseVersionID = "v-none" }},
		{"evidence_source_ids",
			func(r *SuggestionRequest) { r.Content.EvidenceSourceIDs = []string{"src-own", "src-foreign"} },
			func(r *SuggestionRequest) { r.Content.EvidenceSourceIDs = []string{"src-own", "src-none"} }},
	} {
		refusals := []string{}
		for _, edit := range []func(*SuggestionRequest){tc.foreign, tc.missing} {
			store, database, _, _ := scriptedSuggestionStore(scriptState{themeRevision: 2}, false)
			req := validSuggestion()
			edit(&req)
			_, err := store.CreateSearchSuggestion(t.Context(), "ws-a", "actor-a", req)
			wantSearchField(t, err, tc.field)
			if len(database.tx.inserts()) != 0 {
				t.Fatalf("%s: a refused suggestion was written", tc.field)
			}
			encoded, _ := json.Marshal(err)
			refusals = append(refusals, string(encoded))
		}
		if refusals[0] != refusals[1] {
			t.Errorf("%s: foreign %s, missing %s - the refusal tells them apart", tc.field, refusals[0], refusals[1])
		}
	}
}

// FR-104, the other direction: a failed read of a version, a document or a
// material is a storage failure, never "not found" and never a 400.
func TestSuggestionReferenceReadsThatFailAreStorage(t *testing.T) {
	store, _, works, _ := scriptedSuggestionStore(scriptState{themeRevision: 2}, false)
	works.err = errReadFailed
	if _, err := store.CreateSearchSuggestion(t.Context(), "ws-a", "actor-a", validSuggestion()); !errors.Is(err, ErrStorage) {
		t.Fatalf("failed version read = %v, want ErrStorage", err)
	}
	store, _, _, _ = scriptedSuggestionStore(scriptState{themeRevision: 2}, false)
	store.Sources = erringSources{err: errReadFailed}
	req := validSuggestion()
	req.Content.EvidenceSourceIDs = []string{"src-own"}
	if _, err := store.CreateSearchSuggestion(t.Context(), "ws-a", "actor-a", req); !errors.Is(err, ErrStorage) {
		t.Fatalf("failed material read = %v, want ErrStorage", err)
	}
	store, _, _, _ = scriptedSuggestionStore(scriptState{themeRevision: 2}, false)
	store.Works = nil
	if _, err := store.CreateSearchSuggestion(t.Context(), "ws-a", "actor-a", validSuggestion()); !errors.Is(err, ErrStorage) {
		t.Fatalf("no works adapter = %v, want ErrStorage", err)
	}
}

// FR-103 / SC-012: once the workspace deletion has committed, the fence
// refuses create, revise and abandon with ErrNotFound - a 404, not a 503 -
// before any statement runs and before the works adapter is read.
func TestSuggestionWritesAfterWorkspaceDeletionAreNotFound(t *testing.T) {
	store, database, works, sources := scriptedSuggestionStore(scriptState{themeRevision: 2, current: 1}, true)
	req := validSuggestion()
	req.Content.EvidenceSourceIDs = []string{"src-own"}
	for name, write := range map[string]func() error{
		"create": func() error {
			_, err := store.CreateSearchSuggestion(t.Context(), "ws-a", "actor-a", req)
			return err
		},
		"revise": func() error {
			_, err := store.ReviseSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
				SuggestionRevisionRequest{BaseRevision: 1, Content: req.Content})
			return err
		},
		"abandon": func() error {
			_, err := store.DecideSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
				DecisionRequest{Decision: DecisionAbandon, Revision: 1})
			return err
		},
	} {
		if err := write(); !errors.Is(err, ErrNotFound) || errors.Is(err, ErrStorage) {
			t.Errorf("%s after the deletion = %v, want ErrNotFound", name, err)
		}
	}
	if len(database.tx.statements) != 0 || len(works.calls) != 0 || sources.reads != 0 {
		t.Fatalf("after the fence refused: %d statements, works %v, %d material reads",
			len(database.tx.statements), works.calls, sources.reads)
	}
}

// FR-030 / FR-039: a revision may not move the target, may not follow a
// decision and must be based on the current revision; when it is all of
// that, it is written as the next revision with the same target.
func TestReviseSuggestionRules(t *testing.T) {
	content := validSuggestion().Content
	content.Rationale = "补一句依据"
	for field, target := range map[string]SuggestionTarget{
		"work_id":         {WorkID: "work-2"},
		"artifact_id":     {ArtifactID: "doc-2"},
		"base_version_id": {BaseVersionID: "v4"},
		"theme_id":        {ThemeID: "theme-2"},
	} {
		store, database, _, _ := scriptedSuggestionStore(scriptState{themeRevision: 2, current: 1}, false)
		_, err := store.ReviseSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
			SuggestionRevisionRequest{BaseRevision: 1, Target: target, Content: content})
		wantSearchField(t, err, field)
		if len(database.tx.inserts()) != 0 {
			t.Fatalf("%s: a refused revision was written", field)
		}
	}
	for _, tc := range []struct {
		name  string
		state scriptState
		base  int64
		field string
	}{
		{"decided", scriptState{themeRevision: 2, current: 1, decided: true}, 1, "suggestion_id"},
		{"stale", scriptState{themeRevision: 2, current: 2}, 1, "base_revision"},
	} {
		store, database, _, _ := scriptedSuggestionStore(tc.state, false)
		_, err := store.ReviseSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
			SuggestionRevisionRequest{BaseRevision: tc.base, Content: content})
		conflict, ok := errors.AsType[SearchConflict](err)
		if !ok || conflict.Field != tc.field || len(database.tx.inserts()) != 0 {
			t.Fatalf("%s: %v, inserts %v", tc.name, err, database.tx.inserts())
		}
	}
	store, database, _, _ := scriptedSuggestionStore(scriptState{themeRevision: 3, current: 1}, false)
	same := validSuggestion().Target
	view, err := store.ReviseSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
		SuggestionRevisionRequest{BaseRevision: 1, Target: same, Content: content})
	if err != nil {
		t.Fatal(err)
	}
	if view.Revision != 2 || view.SuggestionTarget != same || view.ThemeRevision != 3 || view.Rationale != "补一句依据" {
		t.Fatalf("revision 2 = %+v", view)
	}
	if !strings.Contains(database.tx.statements[0], "FOR UPDATE") {
		t.Fatalf("the first statement after the fence is not the row lock: %q", database.tx.statements[0])
	}
}

// ------------------------------------------------------------ abandon

// T043 / FR-051 / SC-008 in its local form: abandoning writes the decision
// and nothing else - one INSERT, into the decision table, plus the audit -
// and the works adapter is only read, never written. The base need not be
// the latest version.
func TestAbandonWritesOnlyTheDecision(t *testing.T) {
	store, database, works, _ := scriptedSuggestionStore(scriptState{themeRevision: 2, current: 2}, false)
	works.latest["ws-a/work-1/doc-1"] = "v5"
	view, err := store.DecideSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
		DecisionRequest{Decision: DecisionAbandon, Revision: 2, Note: "同事已经改过"})
	if err != nil {
		t.Fatal(err)
	}
	if view.State != StateAbandoned || view.Decision == nil || view.Decision.Decision != DecisionAbandon ||
		view.Decision.SuggestionRevision != 2 || view.Decision.DecidedBy != "actor-a" || view.Decision.Note != "同事已经改过" ||
		view.BaseIsCurrent || len(view.Effects) != 0 {
		t.Fatalf("abandoned = %+v (decision %+v)", view, view.Decision)
	}
	if inserts := database.tx.inserts(); !reflect.DeepEqual(inserts, []string{"content_search_suggestion_decision"}) || !database.tx.committed {
		t.Fatalf("inserts %v, committed %v", inserts, database.tx.committed)
	}
	if !reflect.DeepEqual(works.calls, []string{"Document"}) {
		t.Fatalf("works adapter calls while abandoning: %v, want one Document read", works.calls)
	}
	if !strings.Contains(database.tx.statements[0], "FOR SHARE") {
		t.Fatalf("the first statement after the fence is not the shared row lock: %q", database.tx.statements[0])
	}
}

// T045 / FR-050 / contract §7.2: the revision the person saw must be the
// current one; a suggestion takes one decision; adopt is refused before any
// transaction opens; a suggestion that is not here is not found.
func TestAbandonRefusals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state scriptState
		field string
	}{
		{"stale revision", scriptState{themeRevision: 2, current: 3}, "revision"},
		{"already decided", scriptState{themeRevision: 2, current: 2, decided: true}, "suggestion_id"},
	} {
		store, database, _, _ := scriptedSuggestionStore(tc.state, false)
		_, err := store.DecideSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
			DecisionRequest{Decision: DecisionAbandon, Revision: 2})
		conflict, ok := errors.AsType[SearchConflict](err)
		if !ok || conflict.Field != tc.field || len(database.tx.inserts()) != 0 {
			t.Fatalf("%s: %v, inserts %v", tc.name, err, database.tx.inserts())
		}
	}
	store, database, works, _ := scriptedSuggestionStore(scriptState{themeRevision: 2, current: 2}, false)
	_, err := store.DecideSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1",
		DecisionRequest{Decision: DecisionAdopt, Revision: 2})
	wantSearchField(t, err, "decision")
	if database.begins != 0 || len(works.calls) != 0 {
		t.Fatalf("adopt opened %d transactions and called %v", database.begins, works.calls)
	}
	store, _, _, _ = scriptedSuggestionStore(scriptState{themeRevision: 2}, false)
	if _, err = store.DecideSearchSuggestion(t.Context(), "ws-a", "actor-a", "no-such",
		DecisionRequest{Decision: DecisionAbandon, Revision: 1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("abandoning a missing suggestion = %v, want ErrNotFound", err)
	}
}

// ------------------------------------------------------------ reads

func currentSuggestionRows(rows ...[]any) scriptRows {
	return scriptRows{match: "DISTINCT ON (suggestion_id)", rows: rows}
}

func suggestionRowWith(id, base string, created time.Time) []any {
	row := storedSuggestionRow(1)
	row[1], row[5], row[15] = id, base, created
	return row
}

// FR-035 / FR-104 on read: the single read answers the difference from the
// base version; a document the adapter says is not there is ErrNotFound, a
// failed adapter read ErrStorage.
func TestGetSuggestionReadsTheDocumentThroughTheAdapter(t *testing.T) {
	store, database, works, _ := scriptedSuggestionStore(scriptState{}, false)
	database.reads = []scriptRows{
		currentSuggestionRows(storedSuggestionRow(1)),
		{match: "FROM content_search_theme_revision", rows: [][]any{{"theme-1", int64(2), false}}},
	}
	view, err := store.GetSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1")
	if err != nil {
		t.Fatal(err)
	}
	if view.Diff == nil || !reflect.DeepEqual(*view.Diff, DiffLines(scriptBaseBody, validSuggestion().Content.ProposedBody)) ||
		!view.BaseIsCurrent || view.ThemeChanged || view.State != StateOpen {
		t.Fatalf("read = %+v", view)
	}
	for _, tc := range []struct {
		err, want error
	}{{ErrNotFound, ErrNotFound}, {errReadFailed, ErrStorage}} {
		works.err = tc.err
		if _, err = store.GetSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1"); !errors.Is(err, tc.want) {
			t.Errorf("adapter %v -> %v, want %v", tc.err, err, tc.want)
		}
	}
	works.err = nil
	database.reads = nil
	if _, err = store.GetSearchSuggestion(t.Context(), "ws-a", "actor-a", "sug-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing suggestion = %v, want ErrNotFound", err)
	}
}

// T040 / FR-040 / SC-003 in its local form: a comparison orders by
// created_at then suggestion_id, says whether the bases are the same, opens
// no transaction, and answers a missing member as not found.
func TestCompareSuggestionsOrdersAndReadsOnly(t *testing.T) {
	store, database, works, _ := scriptedSuggestionStore(scriptState{}, false)
	works.bodies["ws-a/work-1/doc-1/v2"] = "更早的正文"
	early, late := scriptNow, scriptNow.Add(time.Minute)
	database.reads = []scriptRows{currentSuggestionRows(
		suggestionRowWith("sug-c", "v3", late),
		suggestionRowWith("sug-b", "v3", early),
		suggestionRowWith("sug-a", "v3", late),
	)}
	comparison, err := store.CompareSearchSuggestions(t.Context(), "ws-a", "actor-a", []string{"sug-a", "sug-b", "sug-c"})
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	for _, view := range comparison.Suggestions {
		order = append(order, view.SuggestionID)
		if view.Diff == nil {
			t.Errorf("%s has no diff in a comparison", view.SuggestionID)
		}
	}
	if !reflect.DeepEqual(order, []string{"sug-b", "sug-a", "sug-c"}) || !comparison.SameBase {
		t.Fatalf("order %v same_base %v", order, comparison.SameBase)
	}
	database.reads = []scriptRows{currentSuggestionRows(suggestionRowWith("sug-a", "v3", early), suggestionRowWith("sug-b", "v2", late))}
	if comparison, err = store.CompareSearchSuggestions(t.Context(), "ws-a", "actor-a", []string{"sug-a", "sug-b"}); err != nil || comparison.SameBase {
		t.Fatalf("different bases: same_base %v, %v", comparison.SameBase, err)
	}
	database.reads = []scriptRows{currentSuggestionRows(suggestionRowWith("sug-a", "v3", early))}
	if _, err = store.CompareSearchSuggestions(t.Context(), "ws-a", "actor-a", []string{"sug-a", "sug-foreign"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing member = %v, want ErrNotFound", err)
	}
	for _, ids := range [][]string{{"sug-a"}, {"a", "b", "c", "d", "e"}} {
		_, err = store.CompareSearchSuggestions(t.Context(), "ws-a", "actor-a", ids)
		wantSearchField(t, err, "ids")
	}
	if database.begins != 0 || works.wrote() {
		t.Fatalf("a comparison opened %d transactions (writes through works: %v)", database.begins, works.wrote())
	}
}

// FR-041: the list filters by derived state after deriving it, and orders by
// created_at, then suggestion_id; it carries no diff.
func TestListSuggestionsFiltersByDerivedState(t *testing.T) {
	store, database, _, _ := scriptedSuggestionStore(scriptState{}, false)
	database.reads = []scriptRows{
		currentSuggestionRows(suggestionRowWith("sug-open", "v3", scriptNow), suggestionRowWith("sug-gone", "v3", scriptNow)),
		{match: "FROM content_search_suggestion_decision", rows: [][]any{
			{"dec-1", "sug-gone", int64(1), "abandon", "", "actor-a", scriptNow},
		}},
	}
	all, err := store.ListSearchSuggestions(t.Context(), "ws-a", "actor-a", SuggestionFilter{})
	if err != nil || len(all) != 2 || all[0].SuggestionID != "sug-gone" || all[0].State != StateAbandoned || all[1].State != StateOpen {
		t.Fatalf("list = %+v, %v", all, err)
	}
	for _, view := range all {
		if view.Diff != nil {
			t.Errorf("%s carries a diff in a list", view.SuggestionID)
		}
	}
	open, err := store.ListSearchSuggestions(t.Context(), "ws-a", "actor-a", SuggestionFilter{State: StateOpen})
	if err != nil || len(open) != 1 || open[0].SuggestionID != "sug-open" {
		t.Fatalf("open = %+v, %v", open, err)
	}
}
