package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/multica-ai/multica/server/internal/content/diagnostics"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workeditor "github.com/multica-ai/multica/server/internal/content/work-editor"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// failSecondSuggestionFence permits the adoption decision and the work-editor
// version transaction, then refuses the effect transaction. It lets this
// handler integration test exercise recovery with the real ApplyBody port.
type failSecondSuggestionFence struct {
	mu     sync.Mutex
	calls  int
	failAt int
}

func (g *failSecondSuggestionFence) LockForContentDiagnosticWrite(_ context.Context, _ pgx.Tx, _ string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if g.failAt > 0 && g.calls >= g.failAt {
		return diagnostics.ErrDenied
	}
	return nil
}

func (g *failSecondSuggestionFence) allowFutureWrites() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failAt = 0
}

// specs/036 PR 2: suggestions over HTTP and the searchWorks adapter (T046
// to T050; FR-100, FR-104, FR-105; SC-003, SC-008, SC-012). The database
// cases run in the handler suite (scripts/test-go-db.sh --suite handler); the
// mapping cases need none. The route and real-middleware cases are in
// cmd/server/content_search_routes_test.go.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md

// ---------------------------------------------------------------- no database

// FR-104, both directions: whatever says "not there" - workspace-core,
// work-editor (missing or malformed) or topic-planning itself - is
// topic-planning's ErrNotFound (a 404), and anything else ErrStorage (a 503).
func TestSearchSuggestionsReadErrorMapsNotFoundAndStorageApart(t *testing.T) {
	for _, err := range []error{workspacecore.ErrNotFound, workeditor.ErrNotFound, workeditor.ErrInvalid, topicplanning.ErrNotFound} {
		got := searchSuggestionsReadError(fmt.Errorf("read: %w", err))
		if !errors.Is(got, topicplanning.ErrNotFound) || errors.Is(got, topicplanning.ErrStorage) {
			t.Errorf("%v -> %v, want ErrNotFound", err, got)
		}
	}
	for _, err := range []error{workeditor.ErrStorage, workeditor.ErrConflict, errors.New("connection reset"), pgx.ErrTxClosed, &pgconn.PgError{Code: "57P01"}} {
		got := searchSuggestionsReadError(err)
		if !errors.Is(got, topicplanning.ErrStorage) || errors.Is(got, topicplanning.ErrNotFound) {
			t.Errorf("%v -> %v, want ErrStorage", err, got)
		}
	}
}

// suggestionNoRowsDB answers every single-row read with "no such row": what
// work-editor sees for a document or version that is gone, a deleted
// workspace's included.
type suggestionNoRowsDB struct{ failingDB }

func (suggestionNoRowsDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return errorRow{err: pgx.ErrNoRows}
}

func workStoreOver(db dbExecutor) *workeditor.Store {
	return &workeditor.Store{DB: workDatabase{dbExecutor: db}}
}

// FR-104 through work-editor's own read code, without a database: a
// document or version that is not there reaches topic-planning as
// ErrNotFound, and so does an id work-editor calls malformed; a failed read
// reaches it as ErrStorage; an adapter with nothing behind it is ErrStorage.
func TestSearchWorksMapsWorkEditorAnswers(t *testing.T) {
	ctx := context.Background()
	gone := searchWorks{store: workStoreOver(suggestionNoRowsDB{})}
	if _, err := gone.Document(ctx, "ws", "actor", "work", "doc"); !errors.Is(err, topicplanning.ErrNotFound) || errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("a missing document = %v, want ErrNotFound", err)
	}
	if _, err := gone.VersionBody(ctx, "ws", "actor", "work", "doc", "v1"); !errors.Is(err, topicplanning.ErrNotFound) || errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("a missing version = %v, want ErrNotFound", err)
	}
	if _, err := gone.VersionBody(ctx, "ws", "actor", "work", "doc", ""); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Errorf("an empty version id = %v, want ErrNotFound", err)
	}
	down := searchWorks{store: workStoreOver(failingDB{})}
	if _, err := down.Document(ctx, "ws", "actor", "work", "doc"); !errors.Is(err, topicplanning.ErrStorage) || errors.Is(err, topicplanning.ErrNotFound) {
		t.Errorf("a failed document read = %v, want ErrStorage", err)
	}
	if _, err := down.VersionBody(ctx, "ws", "actor", "work", "doc", "v1"); !errors.Is(err, topicplanning.ErrStorage) || errors.Is(err, topicplanning.ErrNotFound) {
		t.Errorf("a failed version read = %v, want ErrStorage", err)
	}
	if _, err := (searchWorks{}).Document(ctx, "ws", "actor", "work", "doc"); !errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("document with no store = %v, want ErrStorage", err)
	}
	if _, err := (searchWorks{}).VersionBody(ctx, "ws", "actor", "work", "doc", "v1"); !errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("version with no store = %v, want ErrStorage", err)
	}
}

// ---------------------------------------------------------------- database

// suggestionWorld is a search fixture plus one work with one document on
// version 1, and a theme.
type suggestionWorld struct {
	searchFixture
	work, artifact, version, themeID string
}

const suggestionBaseBody = "羊绒大衣不建议机洗。\n平铺晾干。"

func suggestionWorkspace(t *testing.T, h *Handler, slug string) suggestionWorld {
	t.Helper()
	fx := searchWorkspace(t, slug)
	world := suggestionWorld{
		searchFixture: fx, work: "work-" + fx.card, artifact: "artifact-" + fx.card, version: "version-" + fx.card,
	}
	dbfx.Exec(t, `INSERT INTO content_work (work_id, workspace_id, topic_card_id, snapshot_id, title)
		VALUES ($1, $2, $3, '', '羊绒护理指南')`, world.work, fx.wsID, fx.card)
	dbfx.Exec(t, `INSERT INTO content_artifact (artifact_id, work_id, workspace_id, kind, title, position, draft_body)
		VALUES ($1, $2, $3, 'channel_draft', '小红书稿', 1, $4)`, world.artifact, world.work, fx.wsID, suggestionBaseBody)
	dbfx.Exec(t, `INSERT INTO content_artifact_version (version_id, artifact_id, work_id, workspace_id,
		revision, source, action, body, actor_id) VALUES ($1, $2, $3, $4, 1, 'edited', 'saved', $5, $6)`,
		world.version, world.artifact, world.work, fx.wsID, suggestionBaseBody, testUserID)
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_search_suggestion_effect WHERE workspace_id = $1`,
			`DELETE FROM content_search_suggestion_decision WHERE workspace_id = $1`,
			`DELETE FROM content_search_suggestion_revision WHERE workspace_id = $1`,
			`DELETE FROM content_artifact_version WHERE workspace_id = $1`,
			`DELETE FROM content_artifact WHERE workspace_id = $1`,
			`DELETE FROM content_work WHERE workspace_id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, fx.wsID)
		}
	})
	world.themeID, _ = roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, "")), "theme_id")
	return world
}

// suggestionBody is a valid create body with extra members appended.
func suggestionBody(world suggestionWorld, extra string) string {
	return fmt.Sprintf(`{"work_id":%q,"artifact_id":%q,"base_version_id":%q,"theme_id":%q,
		"target_question":"羊绒大衣能机洗吗","aspects":["topics","body","title"],
		"rationale":"原稿第一段没有正面回答能不能机洗","evidence_source_ids":[%q],
		"proposed_body":"能机洗吗？不建议，只能用羊毛程序。\n平铺晾干。"%s}`,
		world.work, world.artifact, world.version, world.themeID, world.source, extra)
}

func suggestionRows(t *testing.T, table, wsID string) int {
	t.Helper()
	var rows int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, wsID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// SC-003 / FR-035 / FR-041 over HTTP: create (server-written fields dropped),
// read with its difference, list without one, revise, compare, abandon.
func TestContentSearchSuggestionRoundTrip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	world := suggestionWorkspace(t, h, "suggest-roundtrip")
	id, created := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/",
		suggestionBody(world, `,"recorded_by":"someone else","theme_revision":9,"author_kind":"ai"`)), "suggestion_id")
	if created["revision"] != float64(1) || created["theme_revision"] != float64(1) || created["author_kind"] != "human" ||
		created["recorded_by"] != testUserID || created["state"] != "open" || created["base_is_current"] != true ||
		created["theme_changed"] != false || created["decision"] != nil {
		t.Fatalf("created = %v", created)
	}
	if aspects := fmt.Sprint(created["aspects"]); aspects != "[body title topics]" {
		t.Fatalf("aspects = %s, want sorted", aspects)
	}
	got := roiCall(t, h.GetContentSearchSuggestion, world.wsID, "GET", "/", "", "suggestionId", id)
	got.Want(http.StatusOK)
	diff, _ := got.Map()["diff"].(map[string]any)
	if diff["inserted_lines"] != float64(1) || diff["deleted_lines"] != float64(1) {
		t.Fatalf("diff = %v", got.Map()["diff"])
	}
	var list struct {
		Suggestions []map[string]any `json:"suggestions"`
	}
	roiCall(t, h.ListContentSearchSuggestions, world.wsID, "GET", "/?artifact_id="+world.artifact, "").Want(http.StatusOK).JSON(&list)
	if len(list.Suggestions) != 1 || list.Suggestions[0]["diff"] != nil {
		t.Fatalf("list = %v", list.Suggestions)
	}
	_, revised := roiCreated(t, roiCall(t, h.ReviseContentSearchSuggestion, world.wsID, "POST", "/",
		suggestionBody(world, `,"base_revision":1`), "suggestionId", id), "suggestion_id")
	if revised["revision"] != float64(2) {
		t.Fatalf("revised = %v", revised)
	}
	second, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/",
		strings.Replace(suggestionBody(world, ""), "只能用羊毛程序", "可以用羊毛程序", 1)), "suggestion_id")
	compared := roiCall(t, h.CompareContentSearchSuggestions, world.wsID, "GET", "/?ids="+second+","+id, "")
	compared.Want(http.StatusOK)
	if body := compared.Map(); body["same_base"] != true || len(body["suggestions"].([]any)) != 2 {
		t.Fatalf("compare = %v", body)
	}
	assertROIField(t, roiCall(t, h.CompareContentSearchSuggestions, world.wsID, "GET", "/?ids="+id, ""), http.StatusBadRequest, "ids")
	_, abandoned := roiCreated(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/",
		`{"decision":"abandon","revision":2,"note":"同事已经改过"}`, "suggestionId", id), "suggestion_id")
	if abandoned["state"] != "abandoned" {
		t.Fatalf("abandoned = %v", abandoned)
	}
	roiCall(t, h.ListContentSearchSuggestions, world.wsID, "GET", "/?state=abandoned", "").Want(http.StatusOK).JSON(&list)
	if len(list.Suggestions) != 1 || list.Suggestions[0]["suggestion_id"] != id {
		t.Fatalf("abandoned list = %v", list.Suggestions)
	}
	assertROIField(t, roiCall(t, h.ListContentSearchSuggestions, world.wsID, "GET", "/?state=rejected", ""), http.StatusBadRequest, "state")
}

// T064/T065: preflight failures leave no decision, a successful adoption
// appends exactly one suggestion_applied version, and a repeat reports the
// decision conflict before observing that the document has since moved.
func TestContentSearchSuggestionAdoptionPreflightAndRepeat(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	world := suggestionWorkspace(t, h, "suggest-adopt")
	id, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, "")), "suggestion_id")
	if _, err := testPool.Exec(t.Context(), `UPDATE content_artifact SET draft_body='未保存', draft_status='working'
		WHERE workspace_id=$1 AND artifact_id=$2`, world.wsID, world.artifact); err != nil {
		t.Fatal(err)
	}
	assertROIField(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/",
		`{"decision":"adopt","revision":1}`, "suggestionId", id), http.StatusConflict, "draft_status")
	if rows := suggestionRows(t, "content_search_suggestion_decision", world.wsID); rows != 0 {
		t.Fatalf("draft preflight wrote %d decisions", rows)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE content_artifact SET draft_body=$3, draft_status='saved'
		WHERE workspace_id=$1 AND artifact_id=$2`, world.wsID, world.artifact, suggestionBaseBody); err != nil {
		t.Fatal(err)
	}
	adopted := roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/",
		`{"decision":"adopt","revision":1}`, "suggestionId", id)
	adopted.Want(http.StatusCreated)
	if adopted.Map()["state"] != "adopted" {
		t.Fatalf("adoption = %v", adopted.Map())
	}
	var action string
	if err := testPool.QueryRow(t.Context(), `SELECT action FROM content_artifact_version
		WHERE workspace_id=$1 AND artifact_id=$2 ORDER BY revision DESC LIMIT 1`, world.wsID, world.artifact).Scan(&action); err != nil {
		t.Fatal(err)
	}
	if action != "suggestion_applied" {
		t.Fatalf("latest action = %q", action)
	}
	assertROIField(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/",
		`{"decision":"adopt","revision":1}`, "suggestionId", id), http.StatusConflict, "suggestion_id")
}

// T067/T068: the first real ApplyBody transaction can commit before the
// effect ledger is available. A retry (including two concurrent retries)
// replays that one version, records one done effect, and appends no version.
func TestContentSearchSuggestionRetryReplaysTheRealWorkVersion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := feedbackHandler(t)
	newUnrecorded := func(slug string) (*topicplanning.SuggestionStore, suggestionWorld, topicplanning.SuggestionView, *failSecondSuggestionFence) {
		t.Helper()
		world := suggestionWorkspace(t, h, slug)
		id, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, "")), "suggestion_id")
		store := h.searchSuggestionsStore()
		fence := &failSecondSuggestionFence{failAt: 2}
		store.Guard = fence
		view, err := store.DecideSearchSuggestion(ctx, world.wsID, testUserID, id,
			topicplanning.DecisionRequest{Decision: topicplanning.DecisionAdopt, Revision: 1})
		if err != nil || view.State != topicplanning.StateAdoptUnrecorded || view.Decision == nil {
			t.Fatalf("unrecorded adoption = %+v, %v", view, err)
		}
		if versions := suggestionRows(t, "content_artifact_version", world.wsID); versions != 2 {
			t.Fatalf("versions after committed apply = %d, want 2", versions)
		}
		return store, world, view, fence
	}

	store, world, first, fence := newUnrecorded("suggest-retry-real")
	fence.allowFutureWrites()
	recovered, err := store.RetrySearchSuggestionDecision(ctx, world.wsID, testUserID, first.Decision.DecisionID)
	if err != nil || recovered.State != topicplanning.StateAdopted || len(recovered.Effects) != 1 {
		t.Fatalf("recovered adoption = %+v, %v", recovered, err)
	}
	if recovered.Effects[0].VersionID == "" || suggestionRows(t, "content_artifact_version", world.wsID) != 2 {
		t.Fatalf("recovery changed version history: %+v", recovered.Effects)
	}

	store, world, first, fence = newUnrecorded("suggest-retry-real-race")
	fence.allowFutureWrites()
	errs := make([]error, 2)
	var retries sync.WaitGroup
	for i := range errs {
		retries.Go(func() {
			_, errs[i] = store.RetrySearchSuggestionDecision(ctx, world.wsID, testUserID, first.Decision.DecisionID)
		})
	}
	retries.Wait()
	var successes, conflicts int
	for _, retryErr := range errs {
		conflict, isConflict := errors.AsType[topicplanning.SearchConflict](retryErr)
		switch {
		case retryErr == nil:
			successes++
		case isConflict && conflict.Field == "decision_id":
			conflicts++
		default:
			t.Errorf("concurrent retry = %v", retryErr)
		}
	}
	if successes != 1 || conflicts != 1 || suggestionRows(t, "content_artifact_version", world.wsID) != 2 {
		t.Fatalf("retries successes=%d conflicts=%d; versions=%d", successes, conflicts, suggestionRows(t, "content_artifact_version", world.wsID))
	}
}

// T047 / contract §7.1 / FR-100: the decision order, first failure winning.
// A non-member and a suggestion that is not here - missing, or another
// brand's - answer byte for byte alike; then the body, by field; then the
// references; then the revision and state.
func TestContentSearchSuggestionDecisionOrder(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	world := suggestionWorkspace(t, h, "suggest-order")
	other := suggestionWorkspace(t, h, "suggest-order-other")
	id, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, "")), "suggestion_id")
	foreign, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, other.wsID, "POST", "/", suggestionBody(other, "")), "suggestion_id")
	reference := roiCall(t, h.GetContentSearchSuggestion, world.wsID, "GET", "/", "", "suggestionId", "no-such-suggestion")
	reference.Want(http.StatusNotFound)

	// 1. Not a member: every endpoint, the same answer as a missing suggestion.
	outsider := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Suggest outsider", "slug": fmt.Sprintf("suggest-outsider-%d", time.Now().UnixNano()), "description": "x",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id::text = $1`, outsider)
	})
	for name, response := range map[string]*testutil.Response{
		"list":     roiCall(t, h.ListContentSearchSuggestions, outsider, "GET", "/", ""),
		"create":   roiCall(t, h.CreateContentSearchSuggestion, outsider, "POST", "/", suggestionBody(world, "")),
		"compare":  roiCall(t, h.CompareContentSearchSuggestions, outsider, "GET", "/?ids="+id+","+foreign, ""),
		"get":      roiCall(t, h.GetContentSearchSuggestion, outsider, "GET", "/", "", "suggestionId", id),
		"revise":   roiCall(t, h.ReviseContentSearchSuggestion, outsider, "POST", "/", suggestionBody(world, `,"base_revision":1`), "suggestionId", id),
		"decision": roiCall(t, h.DecideContentSearchSuggestion, outsider, "POST", "/", `{"decision":"abandon","revision":1}`, "suggestionId", id),
	} {
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s as a non-member answered differently from a missing suggestion: %s", name, response.Text())
		}
	}

	// 2. The path: another brand's suggestion is a missing one, whatever the body.
	for name, response := range map[string]*testutil.Response{
		"get foreign":                roiCall(t, h.GetContentSearchSuggestion, world.wsID, "GET", "/", "", "suggestionId", foreign),
		"revise foreign":             roiCall(t, h.ReviseContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, `,"base_revision":1`), "suggestionId", foreign),
		"decide foreign":             roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"abandon","revision":1}`, "suggestionId", foreign),
		"decide missing, bad body":   roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"adopt","score":1}`, "suggestionId", "no-such-suggestion"),
		"compare with a foreign one": roiCall(t, h.CompareContentSearchSuggestions, world.wsID, "GET", "/?ids="+id+","+foreign, ""),
	} {
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing suggestion: %s", name, response.Text())
		}
	}

	// 3. The body, by field - before references and before the revision.
	for _, tc := range []struct{ name, body, field string }{
		{"score", suggestionBody(world, `,"score":88`), "score"},
		{"keyword count", suggestionBody(world, `,"keyword_count":5`), "keyword_count"},
		{"aspects", strings.Replace(suggestionBody(world, ""), `["topics","body","title"]`, `["keywords"]`, 1), "aspects"},
		{"rationale", strings.Replace(suggestionBody(world, ""), "原稿第一段没有正面回答能不能机洗", " ", 1), "rationale"},
	} {
		assertROIField(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", tc.body), http.StatusBadRequest, tc.field)
	}
	assertROIField(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"adopt","revision":9}`,
		"suggestionId", id), http.StatusConflict, "revision")

	// 4. References: another brand's theme and version answer like missing ones.
	for _, tc := range []struct{ field, own, foreign, missing string }{
		{"theme_id", world.themeID, other.themeID, "no-such-theme"},
		{"base_version_id", world.version, other.version, "no-such-version"},
	} {
		answers := []*testutil.Response{}
		for _, replacement := range []string{tc.foreign, tc.missing} {
			body := strings.Replace(suggestionBody(world, ""), fmt.Sprintf("%q", tc.own), fmt.Sprintf("%q", replacement), 1)
			response := roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", body)
			assertROIField(t, response, http.StatusBadRequest, tc.field)
			answers = append(answers, response)
		}
		if !sameRefusalApartFromTrace(t, answers[0], answers[1]) {
			t.Errorf("%s: a foreign id and a missing one answer differently", tc.field)
		}
	}
	assertROIField(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/",
		strings.Replace(suggestionBody(world, ""), `能机洗吗？不建议，只能用羊毛程序。\n平铺晾干。`, `羊绒大衣不建议机洗。\n平铺晾干。`, 1)),
		http.StatusBadRequest, "proposed_body")
	assertROIField(t, roiCall(t, h.ReviseContentSearchSuggestion, world.wsID, "POST", "/",
		strings.Replace(suggestionBody(world, `,"base_revision":1`), world.version, other.version, 1), "suggestionId", id),
		http.StatusBadRequest, "base_version_id")

	// 5. Revision and state.
	assertROIField(t, roiCall(t, h.ReviseContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, `,"base_revision":9`),
		"suggestionId", id), http.StatusConflict, "base_revision")
	assertROIField(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"abandon","revision":9}`,
		"suggestionId", id), http.StatusConflict, "revision")
	roiCreated(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"abandon","revision":1}`,
		"suggestionId", id), "suggestion_id")
	assertROIField(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"abandon","revision":1}`,
		"suggestionId", id), http.StatusConflict, "suggestion_id")
	assertROIField(t, roiCall(t, h.ReviseContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, `,"base_revision":1`),
		"suggestionId", id), http.StatusConflict, "suggestion_id")

	if rows := suggestionRows(t, "content_search_suggestion_revision", world.wsID); rows != 1 {
		t.Fatalf("%d suggestion rows after refusals, want the 1 created", rows)
	}
	if rows := suggestionRows(t, "content_search_suggestion_decision", world.wsID); rows != 1 {
		t.Fatalf("%d decisions, want 1", rows)
	}
}

// snapshotRows is every row of one table for one workspace, as JSON.
func snapshotRows(t *testing.T, table, wsID string) string {
	t.Helper()
	var text string
	if err := testPool.QueryRow(t.Context(), `SELECT coalesce(string_agg(row_to_json(r)::text, E'\n' ORDER BY row_to_json(r)::text), '')
		FROM `+table+` r WHERE workspace_id = $1`, wsID).Scan(&text); err != nil {
		t.Fatalf("snapshot %s: %v", table, err)
	}
	return text
}

// T043 / SC-008 / D14-V15 through the real adapter: abandoning a suggestion
// whose base is no longer the latest leaves the work's versions, the
// document's editing copy, the topic card, the brief, the theme, the account
// and its revisions, the materials and the other suggestions exactly as they
// were; it adds one decision and one audit row.
func TestContentSearchAbandonChangesNoWorkOrPlanningRow(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	h := feedbackHandler(t)
	world := suggestionWorkspace(t, h, "suggest-abandon")
	id, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, "")), "suggestion_id")
	dbfx.Exec(t, `INSERT INTO content_artifact_version (version_id, artifact_id, work_id, workspace_id,
		revision, source, action, body, actor_id) VALUES ($1, $2, $3, $4, 2, 'edited', 'saved', '第二版', $5)`,
		world.version+"-2", world.artifact, world.work, world.wsID, testUserID)

	tables := []string{
		"content_work", "content_artifact", "content_artifact_version", "content_topic_card", "content_brief_revision",
		"content_search_theme_revision", "content_search_suggestion_revision", "content_search_suggestion_effect",
		"content_account", "content_account_revision", "content_source",
	}
	before := map[string]string{}
	for _, table := range tables {
		before[table] = snapshotRows(t, table, world.wsID)
	}
	decisions := suggestionRows(t, "content_search_suggestion_decision", world.wsID)
	audits := suggestionRows(t, "content_operation_audit", world.wsID)

	_, abandoned := roiCreated(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/",
		`{"decision":"abandon","revision":1}`, "suggestionId", id), "suggestion_id")
	if abandoned["state"] != "abandoned" || abandoned["base_is_current"] != false {
		t.Fatalf("abandoned = %v", abandoned)
	}
	for _, table := range tables {
		if after := snapshotRows(t, table, world.wsID); after != before[table] {
			t.Errorf("abandoning changed %s", table)
		}
	}
	if got := suggestionRows(t, "content_search_suggestion_decision", world.wsID); got != decisions+1 {
		t.Errorf("%d decisions, want %d", got, decisions+1)
	}
	if got := suggestionRows(t, "content_operation_audit", world.wsID); got != audits+1 {
		t.Errorf("%d audit rows, want %d", got, audits+1)
	}
}

// T048 / FR-103 / FR-104 / FR-105 / SC-012 with the real fence: after the
// workspace is deleted, each searchWorks method answers ErrNotFound - never
// ErrStorage - every endpoint answers 404, not 503, and no suggestion,
// decision or effect row is left; a neighbour keeps its own.
func TestContentSearchSuggestionAdaptersAndEndpointsAfterWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := feedbackHandler(t)
	world := suggestionWorkspace(t, h, "suggest-fence")
	neighbor := suggestionWorkspace(t, h, "suggest-fence-neighbor")
	id, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, "")), "suggestion_id")
	second, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/",
		strings.Replace(suggestionBody(world, ""), "只能用羊毛程序", "可以用羊毛程序", 1)), "suggestion_id")
	roiCreated(t, roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"abandon","revision":1}`,
		"suggestionId", second), "suggestion_id")
	roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, neighbor.wsID, "POST", "/", suggestionBody(neighbor, "")), "suggestion_id")

	works := searchWorks{store: h.workEditorStore()}
	if _, err := works.Document(ctx, world.wsID, testUserID, world.work, world.artifact); err != nil {
		t.Fatalf("document before the deletion: %v", err)
	}
	request := newRequest(http.MethodDelete, "/api/workspaces/"+world.wsID, nil)
	request = withURLParam(request, "id", world.wsID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)

	if _, err := works.Document(ctx, world.wsID, testUserID, world.work, world.artifact); !errors.Is(err, topicplanning.ErrNotFound) || errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("Document after the deletion = %v, want ErrNotFound", err)
	}
	if _, err := works.VersionBody(ctx, world.wsID, testUserID, world.work, world.artifact, world.version); !errors.Is(err, topicplanning.ErrNotFound) || errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("VersionBody after the deletion = %v, want ErrNotFound", err)
	}
	if _, err := works.Apply(ctx, world.wsID, testUserID, topicplanning.SearchApply{
		Key: "deleted-workspace", WorkID: world.work, ArtifactID: world.artifact,
		BaseVersionID: world.version, Body: "不得在已删除工作区写入",
	}); !errors.Is(err, topicplanning.ErrNotFound) || errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("Apply after the deletion = %v, want ErrNotFound", err)
	}
	for name, response := range map[string]*testutil.Response{
		"create":   roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, "")),
		"get":      roiCall(t, h.GetContentSearchSuggestion, world.wsID, "GET", "/", "", "suggestionId", id),
		"revise":   roiCall(t, h.ReviseContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, `,"base_revision":1`), "suggestionId", id),
		"decision": roiCall(t, h.DecideContentSearchSuggestion, world.wsID, "POST", "/", `{"decision":"abandon","revision":1}`, "suggestionId", id),
		"compare":  roiCall(t, h.CompareContentSearchSuggestions, world.wsID, "GET", "/?ids="+id+","+second, ""),
		"list":     roiCall(t, h.ListContentSearchSuggestions, world.wsID, "GET", "/", ""),
	} {
		if response.Code != http.StatusNotFound {
			t.Errorf("%s after the deletion = %d, want 404: %s", name, response.Code, response.Text())
		}
	}
	for _, table := range []string{"content_search_suggestion_revision", "content_search_suggestion_decision", "content_search_suggestion_effect"} {
		if rows := suggestionRows(t, table, world.wsID); rows != 0 {
			t.Errorf("the deleted workspace kept %d %s rows", rows, table)
		}
	}
	if rows := suggestionRows(t, "content_search_suggestion_revision", neighbor.wsID); rows != 1 {
		t.Errorf("the neighbour has %d suggestion rows, want 1", rows)
	}
}

// The fence itself, with the workspace row removed alone (the content rows
// stay, so the adapter would still answer): writes are refused with
// ErrNotFound before the adapter is read, and nothing is written.
func TestContentSearchSuggestionWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	h := feedbackHandler(t)
	world := suggestionWorkspace(t, h, "suggest-fence-row")
	id, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, world.wsID, "POST", "/", suggestionBody(world, "")), "suggestion_id")
	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id::text = $1`, world.wsID); err != nil {
		t.Fatal(err)
	}
	store := h.searchSuggestionsStore()
	req := topicplanning.SuggestionRequest{
		Target: topicplanning.SuggestionTarget{WorkID: world.work, ArtifactID: world.artifact, BaseVersionID: world.version, ThemeID: world.themeID},
		Content: topicplanning.SuggestionContent{
			TargetQuestion: "羊绒大衣能机洗吗", Aspects: []topicplanning.SuggestionAspect{topicplanning.AspectBody},
			Rationale: "依据", ProposedBody: "改过的正文",
		},
	}
	if _, err := store.CreateSearchSuggestion(ctx, world.wsID, testUserID, req); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("create after the delete committed = %v, want ErrNotFound", err)
	}
	if _, err := store.ReviseSearchSuggestion(ctx, world.wsID, testUserID, id,
		topicplanning.SuggestionRevisionRequest{BaseRevision: 1, Content: req.Content}); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("revise after the delete committed = %v, want ErrNotFound", err)
	}
	if _, err := store.DecideSearchSuggestion(ctx, world.wsID, testUserID, id,
		topicplanning.DecisionRequest{Decision: topicplanning.DecisionAbandon, Revision: 1}); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("abandon after the delete committed = %v, want ErrNotFound", err)
	}
	if _, err := store.DecideSearchSuggestion(ctx, world.wsID, testUserID, id,
		topicplanning.DecisionRequest{Decision: topicplanning.DecisionAdopt, Revision: 1}); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("adopt after the delete committed = %v, want ErrNotFound", err)
	}
	if rows := suggestionRows(t, "content_search_suggestion_revision", world.wsID); rows != 1 {
		t.Fatalf("%d suggestion rows after fenced writes, want the 1 from before", rows)
	}
	if rows := suggestionRows(t, "content_search_suggestion_decision", world.wsID); rows != 0 {
		t.Fatalf("%d decisions after fenced writes", rows)
	}
}
