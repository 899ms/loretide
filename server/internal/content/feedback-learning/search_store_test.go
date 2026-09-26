package feedbacklearning

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// The search write and read paths against an in-memory stand-in for the
// statements they run, so every plain `go test` exercises them: the fence
// comes first, nil and 0 reach the column apart, references are refused by
// field, a stale base is a 409, the list is one row per observation. The
// stand-in answers each SELECT with every column production scans, in
// production's order - a stand-in that left one out would pass here and fail
// against PostgreSQL (#276's fixture did exactly that). The real schema -
// CHECKs, the unique index, the SQL ordering - is proven by
// internal/handler's database suite.

type searchMemMetric struct {
	workspace, id, publication, platform, account, metric string
	value                                                 *int64
	unit, window                                          string
	sampled                                               time.Time
	evidence, source, actor                               string
	created                                               time.Time
}

type searchMemObservation struct {
	workspace, id            string
	revision                 int
	voided                   bool
	platform, account, query string
	theme, publication       string
	observed                 time.Time
	conditions, kind         string
	position, depth          *int
	evidence, actor          string
	created                  time.Time
}

type searchMemDB struct {
	mu           sync.Mutex
	metrics      []searchMemMetric
	observations []searchMemObservation
	clock        time.Time
}

func (db *searchMemDB) tick() time.Time {
	db.clock = db.clock.Add(time.Second)
	return db.clock
}

func (db *searchMemDB) Begin(context.Context) (pgx.Tx, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return &searchMemTx{db: db, metrics: slices.Clone(db.metrics), observations: slices.Clone(db.observations)}, nil
}

type searchMemTx struct {
	pgx.Tx
	db           *searchMemDB
	metrics      []searchMemMetric
	observations []searchMemObservation
	done         bool
}

func (tx *searchMemTx) Commit(context.Context) error {
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	if !tx.done {
		tx.db.metrics, tx.db.observations, tx.done = tx.metrics, tx.observations, true
	}
	return nil
}

func (tx *searchMemTx) Rollback(context.Context) error {
	tx.done = true
	return nil
}

// searchMemRow scans values into production's destinations, converting a
// plain string into the named string type a destination has.
type searchMemRow struct {
	values []any
	err    error
}

func (r searchMemRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if r.values == nil {
		return pgx.ErrNoRows
	}
	if len(dest) != len(r.values) {
		return errors.New("column count differs from production's scan")
	}
	for i, target := range dest {
		slot := reflect.ValueOf(target).Elem()
		value := reflect.ValueOf(r.values[i])
		if value.Type() != slot.Type() {
			value = value.Convert(slot.Type())
		}
		slot.Set(value)
	}
	return nil
}

type searchMemRows struct {
	pgx.Rows
	rows [][]any
	next int
}

func (r *searchMemRows) Next() bool { r.next++; return r.next <= len(r.rows) }
func (r *searchMemRows) Scan(dest ...any) error {
	return searchMemRow{values: r.rows[r.next-1]}.Scan(dest...)
}
func (r *searchMemRows) Err() error { return nil }
func (r *searchMemRows) Close()     {}

func (m searchMemMetric) columns() []any {
	return []any{m.id, m.publication, m.platform, m.account, m.metric, m.value, m.unit, m.window,
		m.sampled, m.evidence, m.source, m.actor, m.created}
}

func (o searchMemObservation) columns() []any {
	return []any{o.id, o.revision, o.voided, o.platform, o.account, o.query, o.theme, o.publication,
		o.observed, o.conditions, o.kind, o.position, o.depth, o.evidence, o.actor, o.created}
}

func latestMemObservation(rows []searchMemObservation, workspace, id string) (searchMemObservation, bool) {
	var latest searchMemObservation
	found := false
	for _, row := range rows {
		if row.workspace == workspace && row.id == id && (!found || row.revision > latest.revision) {
			latest, found = row, true
		}
	}
	return latest, found
}

func (tx *searchMemTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	switch {
	case strings.Contains(sql, "INSERT INTO content_search_metric"):
		row := searchMemMetric{
			workspace: args[0].(string), id: args[1].(string), publication: args[2].(string),
			platform: args[3].(string), account: args[4].(string), metric: args[5].(string),
			value: args[6].(*int64), unit: args[7].(string), window: args[8].(string),
			sampled: args[9].(time.Time), evidence: args[10].(string), source: args[11].(string),
			actor: args[12].(string), created: tx.db.tick(),
		}
		tx.metrics = append(tx.metrics, row)
		return searchMemRow{values: []any{row.created}}
	case strings.Contains(sql, "INSERT INTO content_search_rank_observation_revision"):
		row := searchMemObservation{
			workspace: args[0].(string), id: args[1].(string), revision: args[2].(int), voided: args[3].(bool),
			platform: args[4].(string), account: args[5].(string), query: args[6].(string),
			theme: args[7].(string), publication: args[8].(string), observed: args[9].(time.Time),
			conditions: args[10].(string), kind: args[11].(string), position: args[12].(*int),
			depth: args[13].(*int), evidence: args[14].(string), actor: args[15].(string), created: tx.db.tick(),
		}
		// The unique key index (workspace_id, observation_id, revision), which
		// also sees what other transactions have committed meanwhile.
		tx.db.mu.Lock()
		committed := slices.Clone(tx.db.observations)
		tx.db.mu.Unlock()
		for _, existing := range append(committed, tx.observations...) {
			if existing.workspace == row.workspace && existing.id == row.id && existing.revision == row.revision {
				return searchMemRow{err: &pgconn.PgError{Code: "23505"}}
			}
		}
		tx.observations = append(tx.observations, row)
		return searchMemRow{values: []any{row.created}}
	case strings.Contains(sql, "FROM content_search_rank_observation_revision"):
		latest, ok := latestMemObservation(tx.observations, args[0].(string), args[1].(string))
		if !ok {
			return searchMemRow{}
		}
		return searchMemRow{values: latest.columns()}
	}
	panic("unexpected statement: " + sql)
}

func (db *searchMemDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	db.mu.Lock()
	defer db.mu.Unlock()
	return (&searchMemTx{db: db, observations: db.observations}).QueryRow(ctx, sql, args...)
}

func (db *searchMemDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	rows := &searchMemRows{}
	switch {
	case strings.Contains(sql, "FROM content_search_metric"):
		matched := []searchMemMetric{}
		for _, row := range db.metrics {
			if row.workspace == args[0].(string) && row.publication == args[1].(string) {
				matched = append(matched, row)
			}
		}
		// ORDER BY sampled_at DESC, search_metric_id
		sort.SliceStable(matched, func(i, j int) bool {
			if !matched[i].sampled.Equal(matched[j].sampled) {
				return matched[i].sampled.After(matched[j].sampled)
			}
			return matched[i].id < matched[j].id
		})
		for _, row := range matched {
			rows.rows = append(rows.rows, row.columns())
		}
	case strings.Contains(sql, "FROM content_search_rank_observation_revision"):
		workspace, includeVoided := args[0].(string), args[1].(bool)
		theme, publication, query := args[2].(string), args[3].(string), args[4].(string)
		seen := map[string]bool{}
		matched := []searchMemObservation{}
		for _, row := range db.observations {
			if row.workspace != workspace || seen[row.id] {
				continue
			}
			seen[row.id] = true
			latest, _ := latestMemObservation(db.observations, workspace, row.id)
			if (includeVoided || !latest.voided) && (theme == "" || latest.theme == theme) &&
				(publication == "" || latest.publication == publication) && (query == "" || latest.query == query) {
				matched = append(matched, latest)
			}
		}
		// ORDER BY observed_at DESC, observation_id
		sort.SliceStable(matched, func(i, j int) bool {
			if !matched[i].observed.Equal(matched[j].observed) {
				return matched[i].observed.After(matched[j].observed)
			}
			return matched[i].id < matched[j].id
		})
		for _, row := range matched {
			rows.rows = append(rows.rows, row.columns())
		}
	default:
		panic("unexpected statement: " + sql)
	}
	return rows, nil
}

// searchMemGuard is the workspace delete fence: a deleted workspace is
// refused the way workspace-core refuses it.
type searchMemGuard struct{ deleted map[string]bool }

func (g searchMemGuard) LockForContentDiagnosticWrite(_ context.Context, _ pgx.Tx, workspace string) error {
	if g.deleted[workspace] {
		return diagnostics.ErrDenied
	}
	return nil
}

type searchMemAudit struct {
	mu    sync.Mutex
	steps []string
}

func (a *searchMemAudit) AuditTx(_ context.Context, _ pgx.Tx, _ diagnostics.Scope, event diagnostics.Event) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.steps = append(a.steps, event.Step)
	return nil
}
func (a *searchMemAudit) Technical(context.Context, diagnostics.Event) {}

// searchMemRefs answers the three adapters from maps: an id in the map is
// here, an id in failing is a storage failure, anything else is not here.
// calls counts every adapter call, so a test can see the fence came first.
type searchMemRefs struct {
	mu                             sync.Mutex
	publications, accounts, themes map[string]bool
	failing                        map[string]bool
	calls                          int
}

func (r *searchMemRefs) lookup(set map[string]bool, id string) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	switch {
	case r.failing[id]:
		return ErrStorage
	case set[id]:
		return nil
	default:
		return ErrNotFound
	}
}

type searchMemPublications struct{ refs *searchMemRefs }

func (p searchMemPublications) Resolve(_ context.Context, _ string, id string) (string, string, string, error) {
	return "w", "a", "v", p.refs.lookup(p.refs.publications, id)
}

type searchMemAccounts struct{ refs *searchMemRefs }

func (a searchMemAccounts) AccountExists(_ context.Context, _ string, id string) error {
	return a.refs.lookup(a.refs.accounts, id)
}

type searchMemThemes struct{ refs *searchMemRefs }

func (t searchMemThemes) ThemeExists(_ context.Context, _, _ string, id string) error {
	return t.refs.lookup(t.refs.themes, id)
}

type searchFixture struct {
	db     *searchMemDB
	audit  *searchMemAudit
	refs   *searchMemRefs
	guard  searchMemGuard
	store  *SearchStore
	nextID int
}

func newSearchFixture() *searchFixture {
	fx := &searchFixture{
		db:    &searchMemDB{clock: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)},
		audit: &searchMemAudit{},
		refs: &searchMemRefs{
			publications: map[string]bool{"p1": true, "p2": true},
			accounts:     map[string]bool{"acct-1": true},
			themes:       map[string]bool{"theme-1": true},
			failing:      map[string]bool{"broken": true},
		},
		guard: searchMemGuard{deleted: map[string]bool{}},
	}
	fx.store = &SearchStore{
		Store: &Store{
			DB: fx.db, Diagnostics: fx.audit, Guard: fx.guard,
			Publications: searchMemPublications{refs: fx.refs},
			NewID: func() string {
				fx.nextID++
				return "id-" + string(rune('a'+fx.nextID-1))
			},
		},
		Accounts: searchMemAccounts{refs: fx.refs},
		Themes:   searchMemThemes{refs: fx.refs},
		Now:      func() time.Time { return observationNow },
	}
	return fx
}

// ---------------------------------------------------------------- metrics

// T084 / SC-010 / US7 scenario 2: nil is stored as unknown and 0 as a
// confirmed zero, and the two read back different - all the way to JSON.
func TestSearchMetricNilAndZeroReadBackDifferent(t *testing.T) {
	fx := newSearchFixture()
	ctx := t.Context()
	zero := int64(0)
	unknown := validSearchMetric()
	unknown.Value = nil
	confirmed := validSearchMetric()
	confirmed.Value, confirmed.Metric, confirmed.SampledAt = &zero, SearchMetricVisit, "2026-10-01T00:00:00Z"
	for _, input := range []SearchMetricInput{unknown, confirmed} {
		written, err := fx.store.RecordSearchMetric(ctx, "ws", "u1", input)
		if err != nil {
			t.Fatal(err)
		}
		if written.SourceType != SearchSourceManual || written.RecordedBy != "u1" || written.DataOrigin != DataOriginManualOnly {
			t.Fatalf("written = %+v", written)
		}
	}
	if fx.db.metrics[0].value != nil || fx.db.metrics[1].value == nil || *fx.db.metrics[1].value != 0 {
		t.Fatalf("stored values = %v, %v; want NULL and 0", fx.db.metrics[0].value, fx.db.metrics[1].value)
	}
	listed, err := fx.store.ListSearchMetrics(ctx, "ws", "u1", "p1")
	if err != nil || len(listed) != 2 {
		t.Fatalf("list = %+v, %v", listed, err)
	}
	encoded, _ := json.Marshal(listed)
	if !strings.Contains(string(encoded), `"metric":"search_impression","value":null`) ||
		!strings.Contains(string(encoded), `"metric":"search_visit","value":0`) {
		t.Fatalf("unknown and zero do not read back apart: %s", encoded)
	}
	// Newest sample first: the impression was sampled 2026-10-02.
	if listed[0].Metric != SearchMetricImpression || listed[1].Metric != SearchMetricVisit {
		t.Fatalf("order = %s, %s", listed[0].Metric, listed[1].Metric)
	}
	if !slices.Equal(fx.audit.steps, []string{"record-search-metric", "record-search-metric"}) {
		t.Fatalf("audit = %v", fx.audit.steps)
	}
}

// US7 scenario 3: a metric nobody recorded is absent, not 0.
func TestAnUnrecordedSearchMetricIsAbsent(t *testing.T) {
	fx := newSearchFixture()
	listed, err := fx.store.ListSearchMetrics(t.Context(), "ws", "u1", "p2")
	if err != nil || len(listed) != 0 || listed == nil {
		t.Fatalf("list = %#v, %v; want an empty list", listed, err)
	}
	_, err = fx.store.ListSearchMetrics(t.Context(), "ws", "u1", "")
	wantField(t, err, "publication_record_id")
	if _, err = fx.store.ListSearchMetrics(t.Context(), "ws", "u1", "no-such-record"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("list of a record that is not here = %v, want ErrNotFound", err)
	}
}

// T084 / US7 scenario 5 / FR-104: a publication record that is not here -
// missing, another brand's - is refused as ErrNotFound; an account that is
// not here is 400 account_id; an adapter's storage failure stays a storage
// failure. Nothing is written on any of them.
func TestSearchMetricReferences(t *testing.T) {
	fx := newSearchFixture()
	ctx := t.Context()
	for _, tc := range []struct {
		name        string
		publication string
		account     string
		check       func(error) bool
	}{
		{"missing record", "no-such-record", "", func(err error) bool { return errors.Is(err, ErrNotFound) && !errors.Is(err, ErrInvalid) }},
		{"record read fails", "broken", "", func(err error) bool { return errors.Is(err, ErrStorage) }},
		{"missing account", "p1", "no-such-account", func(err error) bool {
			fieldErr, ok := errors.AsType[FieldError](err)
			return ok && fieldErr.Field == "account_id"
		}},
		{"account read fails", "p1", "broken", func(err error) bool { return errors.Is(err, ErrStorage) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := validSearchMetric()
			input.PublicationRecordID, input.AccountID = tc.publication, tc.account
			if _, err := fx.store.RecordSearchMetric(ctx, "ws", "u1", input); !tc.check(err) {
				t.Fatalf("err = %v", err)
			}
		})
	}
	input := validSearchMetric()
	input.AccountID = "acct-1"
	if _, err := fx.store.RecordSearchMetric(ctx, "ws", "u1", input); err != nil {
		t.Fatalf("an existing account is refused: %v", err)
	}
	if len(fx.db.metrics) != 1 || len(fx.audit.steps) != 1 {
		t.Fatalf("%d metrics and %d audit events after four refusals and one write", len(fx.db.metrics), len(fx.audit.steps))
	}
}

// FR-103 / SC-012: after a workspace deletion has committed, every write is
// refused as ErrNotFound (404, not 503) by the fence - before any adapter is
// asked - and leaves nothing.
func TestSearchWritesAreFencedFirst(t *testing.T) {
	fx := newSearchFixture()
	ctx := t.Context()
	written, err := fx.store.RecordRankObservation(ctx, "ws", "u1", validObservation())
	if err != nil {
		t.Fatal(err)
	}
	fx.guard.deleted["ws"] = true
	fx.refs.calls = 0
	if _, err := fx.store.RecordSearchMetric(ctx, "ws", "u1", validSearchMetric()); !errors.Is(err, ErrNotFound) {
		t.Errorf("metric after the deletion = %v, want ErrNotFound", err)
	}
	withRefs := validObservation()
	withRefs.ThemeID, withRefs.AccountID, withRefs.PublicationRecordID = "theme-1", "acct-1", "p1"
	if _, err := fx.store.RecordRankObservation(ctx, "ws", "u1", withRefs); !errors.Is(err, ErrNotFound) {
		t.Errorf("observation after the deletion = %v, want ErrNotFound", err)
	}
	if _, err := fx.store.ReviseRankObservation(ctx, "ws", "u1", written.ObservationID,
		RankObservationRevision{BaseRevision: 1, Voided: true, Input: withRefs}); !errors.Is(err, ErrNotFound) {
		t.Errorf("void after the deletion = %v, want ErrNotFound", err)
	}
	if fx.refs.calls != 0 {
		t.Errorf("%d adapter calls after the fence refused; the fence must come first", fx.refs.calls)
	}
	if len(fx.db.metrics) != 0 || len(fx.db.observations) != 1 || len(fx.audit.steps) != 1 {
		t.Fatalf("after fenced writes: %d metrics, %d observation rows, %d audit events",
			len(fx.db.metrics), len(fx.db.observations), len(fx.audit.steps))
	}
}

// ---------------------------------------------------------------- observations

// T085 / FR-073 / US8 scenario 6: create, correct, void - each a new
// revision, the first never rewritten; a stale base is a 409; the list
// leaves voided observations out unless asked.
func TestRankObservationRevisions(t *testing.T) {
	fx := newSearchFixture()
	ctx := t.Context()
	input := validObservation()
	input.ThemeID = "theme-1"
	first, err := fx.store.RecordRankObservation(ctx, "ws", "u1", input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 || first.Voided || first.Rule != RuleRankSingleObservation || first.RecordedBy != "u1" {
		t.Fatalf("first = %+v", first)
	}
	firstRow := fx.db.observations[0]

	corrected := input
	corrected.Position = intPointer(8)
	second, err := fx.store.ReviseRankObservation(ctx, "ws", "u2", first.ObservationID,
		RankObservationRevision{BaseRevision: 1, Input: corrected})
	if err != nil || second.Revision != 2 || *second.Position != 8 || second.ObservationID != first.ObservationID {
		t.Fatalf("second = %+v, %v", second, err)
	}
	_, err = fx.store.ReviseRankObservation(ctx, "ws", "u2", first.ObservationID,
		RankObservationRevision{BaseRevision: 1, Input: corrected})
	if conflict, ok := errors.AsType[RevisionConflict](err); !ok || conflict.Field != "base_revision" {
		t.Fatalf("stale base = %v, want a 409 on base_revision", err)
	}
	if _, err = fx.store.ReviseRankObservation(ctx, "ws", "u2", "no-such-observation",
		RankObservationRevision{BaseRevision: 1, Input: corrected}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revising a missing observation = %v, want ErrNotFound", err)
	}
	voided, err := fx.store.ReviseRankObservation(ctx, "ws", "u2", first.ObservationID,
		RankObservationRevision{BaseRevision: 2, Voided: true, Input: corrected})
	if err != nil || voided.Revision != 3 || !voided.Voided {
		t.Fatalf("void = %+v, %v", voided, err)
	}
	if !reflect.DeepEqual(fx.db.observations[0], firstRow) || len(fx.db.observations) != 3 {
		t.Fatalf("revision 1 changed or rows were not appended: %+v", fx.db.observations)
	}
	listed, err := fx.store.ListRankObservations(ctx, "ws", "u1", RankObservationFilter{ThemeID: "theme-1"})
	if err != nil || len(listed) != 0 {
		t.Fatalf("default list = %+v, %v; a voided observation is listed", listed, err)
	}
	listed, err = fx.store.ListRankObservations(ctx, "ws", "u1", RankObservationFilter{ThemeID: "theme-1", IncludeVoided: true})
	if err != nil || len(listed) != 1 || listed[0].Revision != 3 || !listed[0].Voided {
		t.Fatalf("include_voided list = %+v, %v", listed, err)
	}
	if !slices.Equal(fx.audit.steps, []string{"record-rank-observation", "revise-rank-observation", "void-rank-observation"}) {
		t.Fatalf("audit = %v", fx.audit.steps)
	}
	got, err := fx.store.GetRankObservation(ctx, "ws", "u1", first.ObservationID)
	if err != nil || got.Revision != 3 {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err = fx.store.GetRankObservation(ctx, "other-ws", "u1", first.ObservationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another brand's observation = %v, want ErrNotFound", err)
	}
}

// T086 / FR-075 / SC-010 / US8 scenario 4: three observations of one theme
// come back as three, newest first, ties by observation_id, each carrying
// the single-observation rule. Nothing combines them.
func TestRankObservationsAreListedOneByOne(t *testing.T) {
	fx := newSearchFixture()
	ctx := t.Context()
	written := []string{}
	for _, observed := range []string{"2026-10-02T21:30:00+08:00", "2026-10-02T22:00:00+08:00", "2026-10-02T21:30:00+08:00"} {
		input := validObservation()
		input.ThemeID, input.ObservedAt = "theme-1", observed
		observation, err := fx.store.RecordRankObservation(ctx, "ws", "u1", input)
		if err != nil {
			t.Fatal(err)
		}
		written = append(written, observation.ObservationID)
	}
	other := validObservation()
	other.Query = "羊绒大衣起球"
	otherObservation, err := fx.store.RecordRankObservation(ctx, "ws", "u1", other)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := fx.store.ListRankObservations(ctx, "ws", "u1", RankObservationFilter{ThemeID: "theme-1"})
	if err != nil || len(listed) != 3 {
		t.Fatalf("list = %+v, %v; want the three", listed, err)
	}
	ids := []string{listed[0].ObservationID, listed[1].ObservationID, listed[2].ObservationID}
	if !slices.Equal(ids, []string{written[1], written[0], written[2]}) || written[0] > written[2] {
		t.Fatalf("order = %v, want newest first then by id", ids)
	}
	for _, observation := range listed {
		if observation.Rule != RuleRankSingleObservation || observation.DataOrigin != DataOriginManualOnly {
			t.Fatalf("observation = %+v", observation)
		}
	}
	byQuery, err := fx.store.ListRankObservations(ctx, "ws", "u1", RankObservationFilter{Query: "羊绒大衣起球"})
	if err != nil || len(byQuery) != 1 || byQuery[0].ObservationID != otherObservation.ObservationID {
		t.Fatalf("by query = %+v, %v", byQuery, err)
	}
	if _, err = fx.store.ListRankObservations(ctx, "ws", "u1", RankObservationFilter{}); err == nil {
		t.Fatal("a list with no filter is answered")
	}
}

// T085 / FR-073 / FR-078: a theme, account or publication record that is not
// here is 400 naming the field; an adapter's storage failure stays a storage
// failure. Nothing is written on any of them.
func TestRankObservationReferences(t *testing.T) {
	fx := newSearchFixture()
	ctx := t.Context()
	for _, tc := range []struct{ field, id string }{
		{"theme_id", "no-such-theme"}, {"account_id", "no-such-account"}, {"publication_record_id", "no-such-record"},
	} {
		input := validObservation()
		switch tc.field {
		case "theme_id":
			input.ThemeID = tc.id
		case "account_id":
			input.AccountID = tc.id
		default:
			input.PublicationRecordID = tc.id
		}
		_, err := fx.store.RecordRankObservation(ctx, "ws", "u1", input)
		wantField(t, err, tc.field)
		input = validObservation()
		input.ThemeID = "broken"
		if _, err = fx.store.RecordRankObservation(ctx, "ws", "u1", input); !errors.Is(err, ErrStorage) {
			t.Fatalf("theme read failure = %v, want ErrStorage", err)
		}
	}
	input := validObservation()
	input.ThemeID, input.AccountID, input.PublicationRecordID = "theme-1", "acct-1", "p1"
	if _, err := fx.store.RecordRankObservation(ctx, "ws", "u1", input); err != nil {
		t.Fatalf("existing references are refused: %v", err)
	}
	if len(fx.db.observations) != 1 {
		t.Fatalf("%d observation rows, want the one accepted", len(fx.db.observations))
	}
}

// Two writers that both read revision 1 and both try to write revision 2:
// the unique key index turns the loser into the same 409 the read check
// gives.
func TestAConcurrentRevisionIsA409(t *testing.T) {
	fx := newSearchFixture()
	ctx := t.Context()
	first, err := fx.store.RecordRankObservation(ctx, "ws", "u1", validObservation())
	if err != nil {
		t.Fatal(err)
	}
	fx.store.BeforeRevisionInsert = func(ctx context.Context, id string) {
		fx.store.BeforeRevisionInsert = nil
		if _, err := fx.store.ReviseRankObservation(ctx, "ws", "u2", id,
			RankObservationRevision{BaseRevision: 1, Input: validObservation()}); err != nil {
			t.Errorf("the other writer = %v", err)
		}
	}
	_, err = fx.store.ReviseRankObservation(ctx, "ws", "u1", first.ObservationID,
		RankObservationRevision{BaseRevision: 1, Input: validObservation()})
	if conflict, ok := errors.AsType[RevisionConflict](err); !ok || conflict.Field != "base_revision" {
		t.Fatalf("the loser = %v, want a 409 on base_revision", err)
	}
	if len(fx.db.observations) != 2 {
		t.Fatalf("%d rows, want revisions 1 and 2 only", len(fx.db.observations))
	}
}

// A store with nothing behind it refuses rather than writing around the
// fence or an adapter.
func TestSearchStoreWithoutItsPartsIsAStorageFailure(t *testing.T) {
	ctx := t.Context()
	var empty *SearchStore
	if _, err := empty.RecordSearchMetric(ctx, "ws", "u1", validSearchMetric()); !errors.Is(err, ErrStorage) {
		t.Errorf("nil store = %v", err)
	}
	fx := newSearchFixture()
	fx.store.Themes = nil
	input := validObservation()
	input.ThemeID = "theme-1"
	if _, err := fx.store.RecordRankObservation(ctx, "ws", "u1", input); !errors.Is(err, ErrStorage) {
		t.Errorf("no theme adapter = %v, want ErrStorage", err)
	}
	fx.store.Accounts = nil
	metric := validSearchMetric()
	metric.AccountID = "acct-1"
	if _, err := fx.store.RecordSearchMetric(ctx, "ws", "u1", metric); !errors.Is(err, ErrStorage) {
		t.Errorf("no account adapter = %v, want ErrStorage", err)
	}
	fx.store.Guard = nil
	if _, err := fx.store.RecordSearchMetric(ctx, "ws", "u1", validSearchMetric()); !errors.Is(err, ErrStorage) {
		t.Errorf("no fence = %v, want ErrStorage", err)
	}
	if len(fx.db.metrics) != 0 || len(fx.db.observations) != 0 {
		t.Fatal("something was written without its parts")
	}
}
