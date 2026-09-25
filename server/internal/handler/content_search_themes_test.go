package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/036 PR 1: search themes over HTTP and the two adapters (T021, T023,
// T024; FR-100, FR-104, FR-105; SC-001, SC-002, SC-012). The database cases
// run in the handler suite (scripts/test-go-db.sh --suite handler); the
// mapping and guard cases need none.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md

// ---------------------------------------------------------------- no database

// FR-104, both directions: whatever says "not there" - workspace-core,
// ip-profile or topic-planning itself - is topic-planning's ErrNotFound (a
// 404), and anything else is ErrStorage (a 503). 035 PR 1 first shipped an
// adapter that answered a deleted workspace as storage; #274 answers a
// storage failure as not found. Neither may happen here.
func TestSearchThemesReadErrorMapsNotFoundAndStorageApart(t *testing.T) {
	for _, err := range []error{workspacecore.ErrNotFound, ipprofile.ErrNotFound, topicplanning.ErrNotFound} {
		got := searchThemesReadError(fmt.Errorf("read: %w", err))
		if !errors.Is(got, topicplanning.ErrNotFound) || errors.Is(got, topicplanning.ErrStorage) {
			t.Errorf("%v -> %v, want ErrNotFound", err, got)
		}
	}
	for _, err := range []error{errors.New("connection reset"), pgx.ErrTxClosed, &pgconn.PgError{Code: "57P01"}} {
		got := searchThemesReadError(err)
		if !errors.Is(got, topicplanning.ErrStorage) || errors.Is(got, topicplanning.ErrNotFound) {
			t.Errorf("%v -> %v, want ErrStorage", err, got)
		}
	}
}

// failingDB answers every statement with a database failure.
type failingDB struct{}

func (failingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("connection reset")
}
func (failingDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("connection reset")
}
func (failingDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return errorRow{err: errors.New("connection reset")}
}

// A material read that fails is a storage failure, never "not found"; an
// adapter with nothing behind it is a storage failure too.
func TestSearchThemeAdaptersAnswerStorageWhenTheReadFails(t *testing.T) {
	ctx := context.Background()
	if exists, err := (searchThemeSources{db: failingDB{}}).Exists(ctx, "ws", "src"); exists || !errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("failed material read = %v, %v; want false, ErrStorage", exists, err)
	}
	if _, err := (searchThemeSources{}).Exists(ctx, "ws", "src"); !errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("material read with no database = %v, want ErrStorage", err)
	}
	if _, err := (searchThemeAccounts{}).Get(ctx, "ws", "acct"); !errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("account read with no service = %v, want ErrStorage", err)
	}
	if _, err := (searchThemeAccounts{}).CurrentPersonaRevision(ctx, "ws", "acct"); !errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("persona read with no service = %v, want ErrStorage", err)
	}
}

// FR-001 / FR-005 / contract §10: the search handler files make no outbound
// request, import no executor, and write no review, delivery or
// publication table.
func TestContentSearchHandlerFilesMakeNoOutboundCallsOrForeignWrites(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(current), "content_search_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		checked++
		source := string(body)
		for _, forbidden := range []string{
			"http.Get(", "http.Post(", "http.Head(", "http.NewRequest", "http.Client", "http.DefaultClient",
			"/content/agent-workflow", "/content/agent-gateway",
		} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s contains %q", filepath.Base(path), forbidden)
			}
		}
		flat := strings.ToUpper(whitespaceRun(source))
		for _, table := range []string{"CONTENT_REVIEW_", "CONTENT_DELIVERY_", "CONTENT_PUBLICATION_"} {
			for _, verb := range []string{"INSERT INTO ", "UPDATE ", "DELETE FROM "} {
				if strings.Contains(flat, verb+table) {
					t.Errorf("%s writes %s", filepath.Base(path), table)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no content_search_*.go handler file found; the guard checked nothing")
	}
}

func whitespaceRun(text string) string { return strings.Join(strings.Fields(text), " ") }

// ---------------------------------------------------------------- database

// searchFixture is one brand the test user owns: a xiaohongshu and a douyin
// account, a material, a topic card with one brief revision.
type searchFixture struct {
	wsID, xhs, douyin, source, card, brief string
}

func searchWorkspace(t *testing.T, slug string) searchFixture {
	t.Helper()
	slug = fmt.Sprintf("%s-%d", slug, time.Now().UnixNano())
	fx := searchFixture{
		xhs: "acct-xhs-" + slug, douyin: "acct-dy-" + slug, source: "src-" + slug,
		card: "card-" + slug, brief: "brief-" + slug,
	}
	fx.wsID = dbfx.Insert(t, "workspace", testutil.Cols{"name": "Search " + slug, "slug": slug, "description": "search theme test"})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, fx.wsID, testUserID)
	dbfx.Exec(t, `INSERT INTO content_account (account_id, workspace_id, platform, display_name)
		VALUES ($1, $2, 'xiaohongshu', '主号'), ($3, $2, 'douyin', '副号')`, fx.xhs, fx.wsID, fx.douyin)
	dbfx.Exec(t, `INSERT INTO content_source (source_id, workspace_id, kind, recorded_by, title)
		VALUES ($1, $2, 'pasted_text', $3, '面料资料')`, fx.source, fx.wsID, testUserID)
	dbfx.Exec(t, `INSERT INTO content_topic_card (topic_card_id, workspace_id, audience_problem_judgment, ip_fit,
		timing, existing_content_relation, evidence_gaps_and_investment, channels, recommended_action)
		VALUES ($1, $2, '', '', '', '', '', '[]'::jsonb, '')`, fx.card, fx.wsID)
	dbfx.Exec(t, `INSERT INTO content_brief_revision (brief_revision_id, topic_card_id, workspace_id, revision,
		audience, core_problem, claim_and_boundaries, channels, format, structure, citation_requirements,
		source_scope, deliverable, time_limit, cost_limit)
		VALUES ($1, $2, $3, 1, '', '', '', '[]'::jsonb, '', '', '', '', '', '', '')`, fx.brief, fx.card, fx.wsID)
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_search_theme_revision WHERE workspace_id = $1`,
			`DELETE FROM content_brief_revision WHERE workspace_id = $1`,
			`DELETE FROM content_topic_card WHERE workspace_id = $1`,
			`DELETE FROM content_source WHERE workspace_id = $1`,
			`DELETE FROM content_account WHERE workspace_id = $1`,
			`DELETE FROM content_operation_audit WHERE workspace_id = $1`,
			`DELETE FROM content_technical_log WHERE workspace_id = $1`,
			`DELETE FROM workspace WHERE id::text = $1`,
		} {
			_, _ = testPool.Exec(background, statement, fx.wsID)
		}
	})
	return fx
}

// searchThemeBody is a valid create body with extra members appended.
func searchThemeBody(fx searchFixture, extra string) string {
	return fmt.Sprintf(`{"name":"羊绒大衣怎么洗","platform":"xiaohongshu","account_id":%q,
		"business_goal":"门店到店预约","questions":["羊绒大衣能机洗吗","羊绒大衣起球怎么办"],
		"keywords":["羊绒 ","羊绒","大衣清洗"],"intent":"solve","origin":"customer_question",
		"origin_note":"9 月私信里有 6 个人问过","source_ids":[%q],"topic_card_ids":[%q],
		"brief_revision_ids":[%q],"note":""%s}`, fx.xhs, fx.source, fx.card, fx.brief, extra)
}

func searchRows(t *testing.T, wsID string) int {
	t.Helper()
	var rows int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_search_theme_revision WHERE workspace_id = $1`, wsID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// SC-001 / SC-002 over HTTP: a theme reads back with every field, keywords
// normalized, search volume and competition unknown with a reason, and no
// rank; a revision and an archive keep the history.
func TestContentSearchThemeRoundTrip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := searchWorkspace(t, "search-roundtrip")
	h := feedbackHandler(t)
	themeID, created := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/api/content-search/themes",
		searchThemeBody(fx, `,"recorded_by":"someone else","data_origin":"ai"`)), "theme_id")
	if created["revision"] != float64(1) || created["recorded_by"] != testUserID || created["data_origin"] != "manual_only" {
		t.Fatalf("created = %v", created)
	}
	if keywords := fmt.Sprint(created["keywords"]); keywords != "[羊绒 大衣清洗]" {
		t.Fatalf("keywords = %s, want trimmed and deduplicated", keywords)
	}
	for _, field := range []string{"search_volume", "competition"} {
		value, _ := created[field].(map[string]any)
		if value["status"] != "unknown" || value["reason"] != "no_data_source" {
			t.Fatalf("%s = %v", field, created[field])
		}
	}
	if _, ok := created["rank"]; ok {
		t.Fatal("a theme answers a rank")
	}

	roiCreated(t, roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/revisions",
		searchThemeBody(fx, `,"base_revision":1`), "themeId", themeID), "theme_id")
	_, archived := roiCreated(t, roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/revisions",
		searchThemeBody(fx, `,"base_revision":2,"voided":true`), "themeId", themeID), "theme_id")
	if archived["revision"] != float64(3) || archived["voided"] != true {
		t.Fatalf("archive = %v", archived)
	}
	var history struct {
		ThemeID   string           `json:"theme_id"`
		Revisions []map[string]any `json:"revisions"`
	}
	roiCall(t, h.ListContentSearchThemeRevisions, fx.wsID, "GET", "/", "", "themeId", themeID).Want(http.StatusOK).JSON(&history)
	if history.ThemeID != themeID || len(history.Revisions) != 3 || history.Revisions[0]["revision"] != float64(3) {
		t.Fatalf("history = %+v", history)
	}
	var list struct {
		Themes []map[string]any `json:"themes"`
	}
	roiCall(t, h.ListContentSearchThemes, fx.wsID, "GET", "/api/content-search/themes", "").Want(http.StatusOK).JSON(&list)
	if len(list.Themes) != 0 {
		t.Fatalf("an archived theme is listed: %v", list.Themes)
	}
	roiCall(t, h.ListContentSearchThemes, fx.wsID, "GET", "/api/content-search/themes?include_archived=true&topic_card_id="+fx.card, "").
		Want(http.StatusOK).JSON(&list)
	if len(list.Themes) != 1 {
		t.Fatalf("include_archived by card = %v", list.Themes)
	}
	assertROIField(t, roiCall(t, h.ListContentSearchThemes, fx.wsID, "GET", "/api/content-search/themes?platform=taobao", ""),
		http.StatusBadRequest, "platform")
}

// T021 / contract §7.1 / FR-100: the decision order, first failure winning.
// A non-member and a theme that is not here - missing, or another brand's -
// answer byte for byte alike on every endpoint; then the path; then the
// body, by field; then the references, by field and without telling a
// foreign id from a missing one; then the revision.
func TestContentSearchThemeDecisionOrder(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := searchWorkspace(t, "search-order")
	other := searchWorkspace(t, "search-order-other")
	h := feedbackHandler(t)
	themeID, _ := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, "")), "theme_id")
	foreignTheme, _ := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, other.wsID, "POST", "/", searchThemeBody(other, "")), "theme_id")
	reference := roiCall(t, h.GetContentSearchTheme, fx.wsID, "GET", "/", "", "themeId", "no-such-theme")
	reference.Want(http.StatusNotFound)

	// 1. Not a member: every endpoint, the same answer as a missing theme.
	outsider := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Search outsider", "slug": fmt.Sprintf("search-outsider-%d", time.Now().UnixNano()), "description": "x",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id::text = $1`, outsider)
	})
	for name, response := range map[string]*testutil.Response{
		"list":      roiCall(t, h.ListContentSearchThemes, outsider, "GET", "/", ""),
		"create":    roiCall(t, h.CreateContentSearchTheme, outsider, "POST", "/", searchThemeBody(fx, "")),
		"get":       roiCall(t, h.GetContentSearchTheme, outsider, "GET", "/", "", "themeId", themeID),
		"revisions": roiCall(t, h.ListContentSearchThemeRevisions, outsider, "GET", "/", "", "themeId", themeID),
		"revise":    roiCall(t, h.ReviseContentSearchTheme, outsider, "POST", "/", searchThemeBody(fx, `,"base_revision":1`), "themeId", themeID),
	} {
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s as a non-member answered differently from a missing theme: %s", name, response.Text())
		}
	}

	// 2. The path: another brand's theme is a missing theme, whatever the body.
	for name, response := range map[string]*testutil.Response{
		"get foreign":              roiCall(t, h.GetContentSearchTheme, fx.wsID, "GET", "/", "", "themeId", foreignTheme),
		"revisions foreign":        roiCall(t, h.ListContentSearchThemeRevisions, fx.wsID, "GET", "/", "", "themeId", foreignTheme),
		"revise foreign":           roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, `,"base_revision":1`), "themeId", foreignTheme),
		"revise missing, bad body": roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/", `{"rank":1}`, "themeId", "no-such-theme"),
	} {
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing theme: %s", name, response.Text())
		}
	}

	// 3. The body, by field - before references and before the revision.
	for _, tc := range []struct{ name, body, field string }{
		{"search volume", searchThemeBody(fx, `,"search_volume":12000`), "search_volume"},
		{"competition", searchThemeBody(fx, `,"competition":"low"`), "competition"},
		{"rank", searchThemeBody(fx, `,"rank":3`), "rank"},
		{"scope", searchThemeBody(fx, `,"scope":"online"`), "scope"},
		{"budget", searchThemeBody(fx, `,"budget":5`), "budget"},
		{"wrong type", strings.Replace(searchThemeBody(fx, ""), `"note":""`, `"note":1`, 1), "note"},
		{"no question or keyword", strings.NewReplacer(`["羊绒大衣能机洗吗","羊绒大衣起球怎么办"]`, `[]`, `["羊绒 ","羊绒","大衣清洗"]`, `[]`).Replace(searchThemeBody(fx, "")), "questions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertROIField(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", tc.body), http.StatusBadRequest, tc.field)
			stale := strings.Replace(tc.body, `"note":`, `"base_revision":9,"source_ids":["foreign"],"note":`, 1)
			stale = strings.Replace(stale, fmt.Sprintf(`"source_ids":[%q],`, fx.source), "", 1)
			assertROIField(t, roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/", stale, "themeId", themeID),
				http.StatusBadRequest, tc.field)
		})
	}

	// 4. References, by field; a foreign id and a missing one alike - even
	// with a stale revision, which comes after.
	for _, tc := range []struct{ field, foreign, missing string }{
		{"source_ids", other.source, "no-such-source"},
		{"topic_card_ids", other.card, "no-such-card"},
		{"brief_revision_ids", other.brief, "no-such-brief"},
		{"account_id", other.xhs, "no-such-account"},
	} {
		own := map[string]string{"source_ids": fx.source, "topic_card_ids": fx.card, "brief_revision_ids": fx.brief, "account_id": fx.xhs}[tc.field]
		answers := []*testutil.Response{}
		for _, id := range []string{tc.foreign, tc.missing} {
			body := strings.Replace(searchThemeBody(fx, `,"base_revision":9`), fmt.Sprintf("%q", own), fmt.Sprintf("%q", id), 1)
			response := roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/", body, "themeId", themeID)
			assertROIField(t, response, http.StatusBadRequest, tc.field)
			answers = append(answers, response)
		}
		if !sameRefusalApartFromTrace(t, answers[0], answers[1]) {
			t.Errorf("%s: a foreign id and a missing one answer differently", tc.field)
		}
	}
	assertROIField(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/",
		strings.Replace(searchThemeBody(fx, ""), fx.xhs, fx.douyin, 1)), http.StatusBadRequest, "platform")

	// 5. The revision.
	assertROIField(t, roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, `,"base_revision":9`),
		"themeId", themeID), http.StatusConflict, "base_revision")
	assertROIField(t, roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, ""),
		"themeId", themeID), http.StatusBadRequest, "base_revision")

	if rows := searchRows(t, fx.wsID); rows != 1 {
		t.Fatalf("%d theme rows after refusals, want the 1 created", rows)
	}
}

// T019 / T023 / FR-103 / FR-104 / SC-012 with the real fence: after the
// workspace row is gone, creating and revising are refused with ErrNotFound
// and leave nothing; each adapter answers "not there" - ErrNotFound for the
// account, "no" for the material - never ErrStorage; the endpoints are 404.
func TestContentSearchThemeWritesAndAdaptersAfterWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	fx := searchWorkspace(t, "search-fence")
	h := feedbackHandler(t)
	themeID, _ := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, "")), "theme_id")

	// The deletion chain removes the brand's rows; the workspace row goes last.
	request := newRequest(http.MethodDelete, "/api/workspaces/"+fx.wsID, nil)
	request = withURLParam(request, "id", fx.wsID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)

	store := h.searchThemesStore()
	content := topicplanning.ThemeContent{
		Name: "x", Platform: "xiaohongshu", AccountID: fx.xhs, Keywords: []string{"羊绒"},
		Intent: topicplanning.IntentSolve, Origin: topicplanning.OriginAuthorizedMaterial, SourceIDs: []string{fx.source},
	}
	if _, err := store.CreateSearchTheme(ctx, fx.wsID, testUserID, content); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("create after the deletion = %v, want ErrNotFound", err)
	}
	if _, err := store.ReviseSearchTheme(ctx, fx.wsID, testUserID, themeID,
		topicplanning.ThemeRevisionRequest{BaseRevision: 1, Content: content}); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("revision after the deletion = %v, want ErrNotFound", err)
	}
	if rows := searchRows(t, fx.wsID); rows != 0 {
		t.Fatalf("%d theme rows after the deletion", rows)
	}

	accounts := searchThemeAccounts{service: h.contentAccountService()}
	if _, err := accounts.Get(ctx, fx.wsID, fx.xhs); !errors.Is(err, topicplanning.ErrNotFound) || errors.Is(err, topicplanning.ErrStorage) {
		t.Errorf("account read after the deletion = %v, want ErrNotFound", err)
	}
	if exists, err := (searchThemeSources{db: h.DB}).Exists(ctx, fx.wsID, fx.source); exists || err != nil {
		t.Errorf("material read after the deletion = %v, %v; want false, nil", exists, err)
	}

	for name, response := range map[string]*testutil.Response{
		"create": roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, "")),
		"revise": roiCall(t, h.ReviseContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, `,"base_revision":1`), "themeId", themeID),
		"get":    roiCall(t, h.GetContentSearchTheme, fx.wsID, "GET", "/", "", "themeId", themeID),
	} {
		if response.Code != http.StatusNotFound {
			t.Errorf("%s after the deletion = %d, want 404: %s", name, response.Code, response.Text())
		}
	}
}

// The fence itself, with the workspace row removed alone (the content rows
// stay): writes are refused with ErrNotFound before anything is read or
// written.
func TestContentSearchThemeWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	fx := searchWorkspace(t, "search-fence-row")
	h := feedbackHandler(t)
	themeID, _ := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, "")), "theme_id")
	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id::text = $1`, fx.wsID); err != nil {
		t.Fatal(err)
	}
	store := h.searchThemesStore()
	content := topicplanning.ThemeContent{
		Name: "x", Platform: "xiaohongshu", Keywords: []string{"羊绒"},
		Intent: topicplanning.IntentSolve, Origin: topicplanning.OriginManualKeyword,
	}
	if _, err := store.CreateSearchTheme(ctx, fx.wsID, testUserID, content); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("create after the delete committed = %v, want ErrNotFound", err)
	}
	if _, err := store.ReviseSearchTheme(ctx, fx.wsID, testUserID, themeID,
		topicplanning.ThemeRevisionRequest{BaseRevision: 1, Content: content}); !errors.Is(err, topicplanning.ErrNotFound) {
		t.Fatalf("revision after the delete committed = %v, want ErrNotFound", err)
	}
	if rows := searchRows(t, fx.wsID); rows != 1 {
		t.Fatalf("%d theme rows after fenced writes, want the 1 from before", rows)
	}
}

// FR-105 / SC-012: deleting a workspace removes its themes and none of a
// neighbour's.
func TestDeleteWorkspaceRemovesSearchThemes(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	target := searchWorkspace(t, "search-delete-target")
	neighbor := searchWorkspace(t, "search-delete-neighbor")
	h := feedbackHandler(t)
	for _, fx := range []searchFixture{target, neighbor} {
		roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx, "")), "theme_id")
	}
	request := newRequest(http.MethodDelete, "/api/workspaces/"+target.wsID, nil)
	request = withURLParam(request, "id", target.wsID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)
	if rows := searchRows(t, target.wsID); rows != 0 {
		t.Errorf("the deleted workspace kept %d theme rows", rows)
	}
	if rows := searchRows(t, neighbor.wsID); rows != 1 {
		t.Errorf("the neighbour has %d theme rows, want 1", rows)
	}
}
