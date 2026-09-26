package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	ipprofile "github.com/multica-ai/multica/server/internal/content/ip-profile"
	topicplanning "github.com/multica-ai/multica/server/internal/content/topic-planning"
	workspacecore "github.com/multica-ai/multica/server/internal/content/workspace-core"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/036 PR 4: search metrics and ranking observations over HTTP and the
// three adapters they read through (T089, T090, T092; FR-100, FR-104,
// FR-105, FR-107; SC-009, SC-010, SC-012). The database cases run in the
// handler suite (scripts/test-go-db.sh --suite handler); the mapping cases
// need none.
//
// Contract: specs/036-search-optimization/contracts/search-optimization.md

// ---------------------------------------------------------------- no database

// FR-104, both directions: whatever says "not there" - workspace-core,
// topic-planning or feedback-learning itself - is feedback-learning's
// ErrNotFound (a 404), and anything else is ErrStorage (a 503).
func TestSearchObservationsReadErrorMapsNotFoundAndStorageApart(t *testing.T) {
	if searchObservationsReadError(nil) != nil {
		t.Fatal("nil is not nil")
	}
	for _, err := range []error{workspacecore.ErrNotFound, topicplanning.ErrNotFound, topicplanning.ErrInvalid,
		feedbacklearning.ErrNotFound} {
		got := searchObservationsReadError(fmt.Errorf("read: %w", err))
		if !errors.Is(got, feedbacklearning.ErrNotFound) || errors.Is(got, feedbacklearning.ErrStorage) {
			t.Errorf("%v -> %v, want ErrNotFound", err, got)
		}
	}
	for _, err := range []error{errors.New("connection reset"), pgx.ErrTxClosed, &pgconn.PgError{Code: "57P01"},
		topicplanning.ErrStorage} {
		got := searchObservationsReadError(err)
		if !errors.Is(got, feedbacklearning.ErrStorage) || errors.Is(got, feedbacklearning.ErrNotFound) {
			t.Errorf("%v -> %v, want ErrStorage", err, got)
		}
	}
}

// noRowsDB answers every row read with "no such row", which is what a theme,
// account or publication record of a deleted workspace reads as.
type noRowsDB struct{ failingDB }

func (noRowsDB) QueryRow(context.Context, string, ...any) pgx.Row {
	return errorRow{err: pgx.ErrNoRows}
}

// failingAccountStore is ip-profile's account read answering err.
type failingAccountStore struct {
	ipprofile.Store
	err error
}

func (s failingAccountStore) GetAccount(context.Context, string, string) (ipprofile.Account, error) {
	return ipprofile.Account{}, s.err
}

// T090 / FR-104 at each adapter, both directions, through the adapter
// itself: a theme, an account or a publication record that is not there is
// ErrNotFound; a read that fails is ErrStorage; an adapter with nothing
// behind it is ErrStorage.
func TestSearchObservationAdaptersMapNotFoundAndStorageApart(t *testing.T) {
	ctx := context.Background()
	themes := func(db dbExecutor) searchThemes {
		return searchThemes{store: &topicplanning.Store{DB: topicDatabase{dbExecutor: db}}}
	}
	if err := themes(noRowsDB{}).ThemeExists(ctx, "ws", "u1", "theme-1"); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Errorf("theme not there = %v, want ErrNotFound", err)
	}
	if err := themes(failingDB{}).ThemeExists(ctx, "ws", "u1", "theme-1"); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("theme read fails = %v, want ErrStorage", err)
	}
	if err := (searchThemes{}).ThemeExists(ctx, "ws", "u1", "theme-1"); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("theme adapter with no store = %v, want ErrStorage", err)
	}

	accounts := func(err error) roiAccounts {
		return roiAccounts{service: &ipprofile.Service{Store: failingAccountStore{err: err}}}
	}
	if err := accounts(ipprofile.ErrNotFound).AccountExists(ctx, "ws", "acct"); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Errorf("account not there = %v, want ErrNotFound", err)
	}
	if err := accounts(errors.New("connection reset")).AccountExists(ctx, "ws", "acct"); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("account read fails = %v, want ErrStorage", err)
	}
	if err := (roiAccounts{}).AccountExists(ctx, "ws", "acct"); !errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("account adapter with no service = %v, want ErrStorage", err)
	}

	// 027's publication adapter answers every failed read as "not found":
	// Issue #274, reused here unchanged (plan.md). Only the not-there
	// direction is asserted.
	if _, _, _, err := (feedbackPublications{db: noRowsDB{}}).Resolve(ctx, "ws", "pub"); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Errorf("publication record not there = %v, want ErrNotFound", err)
	}
}

// T092: the store is wired with all three adapters.
func TestFeedbackSearchStoreHasItsAdapters(t *testing.T) {
	store := (&Handler{}).feedbackSearchStore()
	if store.Store == nil || store.Publications == nil || store.Accounts == nil || store.Themes == nil {
		t.Fatalf("store = %+v", store)
	}
	if _, ok := store.Themes.(searchThemes); !ok {
		t.Fatalf("themes adapter = %T", store.Themes)
	}
	if _, ok := store.Accounts.(roiAccounts); !ok {
		t.Fatalf("accounts adapter = %T", store.Accounts)
	}
	if _, ok := store.Publications.(feedbackPublications); !ok {
		t.Fatalf("publications adapter = %T", store.Publications)
	}
}

// ---------------------------------------------------------------- database

// searchObservationFixture is searchWorkspace plus one publication record.
type searchObservationFixture struct {
	searchFixture
	publication string
}

func searchObservationWorkspace(t *testing.T, slug string) searchObservationFixture {
	t.Helper()
	fx := searchObservationFixture{searchFixture: searchWorkspace(t, slug)}
	fx.publication = "pub-" + fx.card
	dbfx.Exec(t, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at)
		VALUES ($1,$2,'w','a','','xiaohongshu','verified_published',$3,'https://example.invalid/p/1', now())`,
		fx.publication, fx.wsID, testUserID)
	// Registered after searchWorkspace's, so it runs first.
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_search_metric WHERE workspace_id = $1`,
			`DELETE FROM content_search_rank_observation_revision WHERE workspace_id = $1`,
			`DELETE FROM content_publication_record WHERE workspace_id = $1`,
		} {
			_, _ = testPool.Exec(background, statement, fx.wsID)
		}
	})
	return fx
}

func searchMetricBody(publication, value, extra string) string {
	return fmt.Sprintf(`{"publication_record_id":%q,"platform":"xiaohongshu","account_id":"",
		"metric":"search_impression","value":%s,"unit":"次","stat_window":"发布后 7 天累计",
		"sampled_at":"2020-10-02T13:30:00Z","evidence_note":"笔记数据页截图 0928-1.png"%s}`, publication, value, extra)
}

func rankObservationBody(themeID, observedAt, extra string) string {
	return fmt.Sprintf(`{"platform":"xiaohongshu","account_id":"","query":" 羊绒大衣能机洗吗 ","theme_id":%q,
		"publication_record_id":"","observed_at":%q,"conditions":"自己手机，未登录，定位上海，综合排序",
		"result_kind":"position","position":7,"scanned_depth":null,"evidence_note":"截图 1002-2130.png"%s}`,
		themeID, observedAt, extra)
}

func searchObservationRows(t *testing.T, table, wsID string) int {
	t.Helper()
	var rows int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM `+table+` WHERE workspace_id = $1`, wsID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// SC-009 / SC-010 / US7 over HTTP: null and 0 read back different; the
// server writes the source; stat_window, a metric outside the two and search
// volume are refused by name; a record that is not this brand's answers the
// same as a missing one.
func TestContentSearchMetricRoundTrip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := searchObservationWorkspace(t, "search-metric")
	other := searchObservationWorkspace(t, "search-metric-other")
	h := feedbackHandler(t)

	_, unknown := roiCreated(t, roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/",
		searchMetricBody(fx.publication, "null", `,"source_type":"csv_import","recorded_by":"someone"`)), "search_metric_id")
	if unknown["value"] != nil || unknown["source_type"] != "manual" || unknown["recorded_by"] != testUserID ||
		unknown["data_origin"] != "manual_only" || unknown["stat_window"] != "发布后 7 天累计" {
		t.Fatalf("recorded = %v", unknown)
	}
	roiCreated(t, roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/",
		strings.Replace(searchMetricBody(fx.publication, "0", ""), "search_impression", "search_visit", 1)), "search_metric_id")

	var list struct {
		PublicationRecordID string           `json:"publication_record_id"`
		Metrics             []map[string]any `json:"metrics"`
	}
	roiCall(t, h.ListContentSearchMetrics, fx.wsID, "GET", "/api/content-search/metrics?publication_record_id="+fx.publication, "").
		Want(http.StatusOK).JSON(&list)
	if len(list.Metrics) != 2 {
		t.Fatalf("metrics = %v", list.Metrics)
	}
	values := map[any]any{}
	for _, metric := range list.Metrics {
		values[metric["metric"]] = metric["value"]
		if _, present := metric["value"]; !present {
			t.Fatalf("a metric has no value member: %v", metric)
		}
	}
	if values["search_impression"] != nil || values["search_visit"] != float64(0) {
		t.Fatalf("null and 0 read back as %v", values)
	}
	var stored struct{ nulls, zeros int }
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE value IS NULL), count(*) FILTER (WHERE value = 0)
		FROM content_search_metric WHERE workspace_id = $1`, fx.wsID).Scan(&stored.nulls, &stored.zeros); err != nil {
		t.Fatal(err)
	}
	if stored.nulls != 1 || stored.zeros != 1 {
		t.Fatalf("stored %d NULL and %d zero, want one each", stored.nulls, stored.zeros)
	}

	for _, tc := range []struct{ name, body, field string }{
		{"no stat_window", strings.Replace(searchMetricBody(fx.publication, "1", ""), `"发布后 7 天累计"`, `""`, 1), "stat_window"},
		{"search_rank", strings.Replace(searchMetricBody(fx.publication, "1", ""), "search_impression", "search_rank", 1), "metric"},
		{"search volume member", searchMetricBody(fx.publication, "1", `,"search_volume":12000`), "search_volume"},
		{"competition member", searchMetricBody(fx.publication, "1", `,"competition":"low"`), "competition"},
		{"negative", searchMetricBody(fx.publication, "-1", ""), "value"},
		{"zhihu", strings.Replace(searchMetricBody(fx.publication, "1", ""), `"platform":"xiaohongshu"`, `"platform":"zhihu"`, 1), "platform"},
		{"wrong type", searchMetricBody(fx.publication, `"1"`, ""), "value"},
		{"missing account", strings.Replace(searchMetricBody(fx.publication, "1", ""), `"account_id":""`, `"account_id":"no-such-account"`, 1), "account_id"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertROIField(t, roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/", tc.body), http.StatusBadRequest, tc.field)
		})
	}
	assertROIField(t, roiCall(t, h.ListContentSearchMetrics, fx.wsID, "GET", "/api/content-search/metrics", ""),
		http.StatusBadRequest, "publication_record_id")

	missing := roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/", searchMetricBody("no-such-record", "1", ""))
	missing.Want(http.StatusNotFound)
	foreign := roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/", searchMetricBody(other.publication, "1", ""))
	foreign.Want(http.StatusNotFound)
	if !sameRefusalApartFromTrace(t, missing, foreign) {
		t.Error("another brand's publication record answers differently from a missing one")
	}
	foreignList := roiCall(t, h.ListContentSearchMetrics, fx.wsID, "GET",
		"/api/content-search/metrics?publication_record_id="+other.publication, "")
	foreignList.Want(http.StatusNotFound)
	if !sameRefusalApartFromTrace(t, foreignList, missing) {
		t.Error("listing another brand's record answers differently from a missing one")
	}
	if rows := searchObservationRows(t, "content_search_metric", fx.wsID); rows != 2 {
		t.Fatalf("%d metric rows after refusals, want the 2 recorded", rows)
	}
}

// SC-009 / SC-010 / US8 over HTTP: three observations of one theme come
// back as three, newest first, each with the single-observation rule and no
// summary; a correction and a void are revisions; the list needs a filter.
func TestContentSearchRankObservationRoundTrip(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := searchObservationWorkspace(t, "search-rank")
	other := searchObservationWorkspace(t, "search-rank-other")
	h := feedbackHandler(t)
	themeID, _ := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx.searchFixture, "")), "theme_id")
	foreignTheme, _ := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, other.wsID, "POST", "/", searchThemeBody(other.searchFixture, "")), "theme_id")

	ids := []string{}
	for _, observed := range []string{"2020-10-02T21:30:00+08:00", "2020-10-03T21:30:00+08:00", "2020-10-01T21:30:00+08:00"} {
		id, created := roiCreated(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/",
			rankObservationBody(themeID, observed, `,"rule":"x","data_origin":"ai"`)), "observation_id")
		if created["revision"] != float64(1) || created["rule"] != "rank.single_observation" ||
			created["data_origin"] != "manual_only" || created["query"] != "羊绒大衣能机洗吗" {
			t.Fatalf("created = %v", created)
		}
		ids = append(ids, id)
	}
	var list struct {
		Observations []map[string]any `json:"observations"`
	}
	roiCall(t, h.ListContentSearchRankObservations, fx.wsID, "GET", "/api/content-search/rank-observations?theme_id="+themeID, "").
		Want(http.StatusOK).JSON(&list)
	if len(list.Observations) != 3 || list.Observations[0]["observation_id"] != ids[1] ||
		list.Observations[1]["observation_id"] != ids[0] || list.Observations[2]["observation_id"] != ids[2] {
		t.Fatalf("list = %v", list.Observations)
	}
	for _, observation := range list.Observations {
		if observation["rule"] != "rank.single_observation" {
			t.Fatalf("an observation without the rule: %v", observation)
		}
		for _, summary := range []string{"avg", "best", "median", "current_rank", "rank", "best_position"} {
			if _, present := observation[summary]; present {
				t.Fatalf("an observation answers %s", summary)
			}
		}
	}

	// A correction, then a void; the list drops the voided one by default.
	roiCreated(t, roiCall(t, h.ReviseContentSearchRankObservation, fx.wsID, "POST", "/",
		rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", `,"base_revision":1,"position":8`), "observationId", ids[0]), "observation_id")
	assertROIField(t, roiCall(t, h.ReviseContentSearchRankObservation, fx.wsID, "POST", "/",
		rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", `,"base_revision":1`), "observationId", ids[0]),
		http.StatusConflict, "base_revision")
	_, voided := roiCreated(t, roiCall(t, h.ReviseContentSearchRankObservation, fx.wsID, "POST", "/",
		rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", `,"base_revision":2,"voided":true`), "observationId", ids[0]), "observation_id")
	if voided["revision"] != float64(3) || voided["voided"] != true {
		t.Fatalf("void = %v", voided)
	}
	roiCall(t, h.ListContentSearchRankObservations, fx.wsID, "GET", "/api/content-search/rank-observations?theme_id="+themeID, "").
		Want(http.StatusOK).JSON(&list)
	if len(list.Observations) != 2 {
		t.Fatalf("default list after the void = %d observations, want 2", len(list.Observations))
	}
	roiCall(t, h.ListContentSearchRankObservations, fx.wsID, "GET",
		"/api/content-search/rank-observations?include_voided=true&query=%E7%BE%8A%E7%BB%92%E5%A4%A7%E8%A1%A3%E8%83%BD%E6%9C%BA%E6%B4%97%E5%90%97", "").
		Want(http.StatusOK).JSON(&list)
	if len(list.Observations) != 3 {
		t.Fatalf("include_voided by query = %d observations, want 3", len(list.Observations))
	}
	var revisionOne string
	if err := testPool.QueryRow(t.Context(), `SELECT row_to_json(r)::text FROM content_search_rank_observation_revision r
		WHERE workspace_id = $1 AND observation_id = $2 AND revision = 1`, fx.wsID, ids[0]).Scan(&revisionOne); err != nil ||
		!strings.Contains(revisionOne, `"position":7`) {
		t.Fatalf("revision 1 = %s, %v; it must be untouched", revisionOne, err)
	}
	assertROIField(t, roiCall(t, h.ListContentSearchRankObservations, fx.wsID, "GET", "/api/content-search/rank-observations", ""),
		http.StatusBadRequest, "theme_id")

	// Field rules and references, by field; a foreign theme and a missing
	// one alike.
	for _, tc := range []struct{ name, body, field string }{
		{"no conditions", strings.Replace(rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", ""), `"自己手机，未登录，定位上海，综合排序"`, `""`, 1), "conditions"},
		{"no evidence", strings.Replace(rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", ""), `"截图 1002-2130.png"`, `""`, 1), "evidence_note"},
		{"no observed_at", rankObservationBody(themeID, "", ""), "observed_at"},
		{"future", rankObservationBody(themeID, time.Now().Add(time.Hour).UTC().Format(time.RFC3339), ""), "observed_at"},
		{"both integers", rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", `,"scanned_depth":30`), "scanned_depth"},
		{"rank member", rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", `,"rank":7`), "rank"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertROIField(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/", tc.body), http.StatusBadRequest, tc.field)
		})
	}
	answers := []*testutil.Response{}
	for _, theme := range []string{foreignTheme, "no-such-theme"} {
		response := roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/", rankObservationBody(theme, "2020-10-02T21:30:00+08:00", ""))
		assertROIField(t, response, http.StatusBadRequest, "theme_id")
		answers = append(answers, response)
	}
	if !sameRefusalApartFromTrace(t, answers[0], answers[1]) {
		t.Error("a foreign theme and a missing one answer differently")
	}
	withRecord := strings.Replace(rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", ""),
		`"publication_record_id":""`, fmt.Sprintf(`"publication_record_id":%q`, other.publication), 1)
	assertROIField(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/", withRecord), http.StatusBadRequest, "publication_record_id")
	withAccount := strings.Replace(rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", ""),
		`"account_id":""`, fmt.Sprintf(`"account_id":%q`, other.xhs), 1)
	assertROIField(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/", withAccount), http.StatusBadRequest, "account_id")

	// The accepting side of each reference.
	all := strings.NewReplacer(`"publication_record_id":""`, fmt.Sprintf(`"publication_record_id":%q`, fx.publication),
		`"account_id":""`, fmt.Sprintf(`"account_id":%q`, fx.xhs)).Replace(rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", ""))
	roiCreated(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/", all), "observation_id")
	if rows := searchObservationRows(t, "content_search_rank_observation_revision", fx.wsID); rows != 6 {
		t.Fatalf("%d observation rows, want 3 created + 2 revisions + 1", rows)
	}
}

// T091 / SC-014 / D14-V15: one real-handler path from a manual search theme,
// through suggestion adoption and a fresh human review, to a publication
// record and the two search observations. The publication resolver must still
// point at the adopted v2; metrics and observations remain attached to their
// actual publication/theme, not to a fabricated cross-module port.
func TestContentSearchLifecycleFromThemeToPublicationObservation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := searchWorkspace(t, "search-lifecycle")
	h := reviewHandler(t)

	// These are all real records made via their HTTP handlers. Cleanup is local
	// to this integration fixture and runs before searchWorkspace removes its
	// topic card, account, and workspace.
	t.Cleanup(func() {
		ctx := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_search_metric WHERE workspace_id=$1`,
			`DELETE FROM content_search_rank_observation_revision WHERE workspace_id=$1`,
			`DELETE FROM content_publication_record WHERE workspace_id=$1`,
			`DELETE FROM content_delivery_task WHERE workspace_id=$1`,
			`DELETE FROM content_review_transition WHERE workspace_id=$1`,
			`DELETE FROM content_review_request WHERE workspace_id=$1`,
			`DELETE FROM content_search_suggestion_effect WHERE workspace_id=$1`,
			`DELETE FROM content_search_suggestion_decision WHERE workspace_id=$1`,
			`DELETE FROM content_search_suggestion_revision WHERE workspace_id=$1`,
			`DELETE FROM content_artifact_version WHERE workspace_id=$1`,
			`DELETE FROM content_artifact WHERE workspace_id=$1`,
			`DELETE FROM content_work WHERE workspace_id=$1`,
		} {
			_, _ = testPool.Exec(ctx, statement, fx.wsID)
		}
	})

	themeID, theme := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, http.MethodPost,
		"/api/content-search/themes", searchThemeBody(fx, "")), "theme_id")
	topicCardIDs, ok := theme["topic_card_ids"].([]any)
	if !ok || theme["account_id"] != fx.xhs || len(topicCardIDs) != 1 || topicCardIDs[0] != fx.card {
		t.Fatalf("theme references = %v, want fixture account %s and topic card %s", theme, fx.xhs, fx.card)
	}

	workID, _ := roiCreated(t, roiCall(t, h.CreateContentWork, fx.wsID, http.MethodPost,
		"/api/content-works", fmt.Sprintf(`{"topic_card_id":%q,"title":"羊绒护理指南"}`, fx.card)), "work_id")
	artifactID, _ := roiCreated(t, roiCall(t, h.CreateContentArtifact, fx.wsID, http.MethodPost,
		"/api/content-works/"+workID+"/artifacts", `{"kind":"channel_draft","title":"小红书稿","position":1}`,
		"id", workID), "artifact_id")
	roiCall(t, h.PatchContentArtifact, fx.wsID, http.MethodPatch, "/api/content-works/"+workID+"/artifacts/"+artifactID,
		fmt.Sprintf(`{"draft_body":%q}`, suggestionBaseBody), "id", workID, "artifactId", artifactID).Want(http.StatusOK)
	version1, versionOne := roiCreated(t, roiCall(t, h.SaveContentArtifactVersion, fx.wsID, http.MethodPost,
		"/api/content-works/"+workID+"/artifacts/"+artifactID+"/versions", "", "id", workID, "artifactId", artifactID), "version_id")
	if versionOne["revision"] != float64(1) || versionOne["body"] != suggestionBaseBody {
		t.Fatalf("initial version = %v", versionOne)
	}

	world := suggestionWorld{searchFixture: fx, work: workID, artifact: artifactID, version: version1, themeID: themeID}
	suggestionID, _ := roiCreated(t, roiCall(t, h.CreateContentSearchSuggestion, fx.wsID, http.MethodPost,
		"/api/content-search/suggestions", suggestionBody(world, "")), "suggestion_id")
	adopted := roiCall(t, h.DecideContentSearchSuggestion, fx.wsID, http.MethodPost,
		"/api/content-search/suggestions/"+suggestionID+"/decisions", `{"decision":"adopt","revision":1}`,
		"suggestionId", suggestionID).Want(http.StatusCreated)
	var adoption struct {
		Effects []struct {
			Outcome   string `json:"outcome"`
			VersionID string `json:"version_id"`
		} `json:"effects"`
	}
	adopted.JSON(&adoption)
	if len(adoption.Effects) != 1 || adoption.Effects[0].Outcome != "done" || adoption.Effects[0].VersionID == "" {
		t.Fatalf("adoption effects = %+v", adoption.Effects)
	}
	version2 := adoption.Effects[0].VersionID
	var revision2 int
	var body2 string
	if err := testPool.QueryRow(t.Context(), `SELECT revision, body FROM content_artifact_version
		WHERE workspace_id=$1 AND artifact_id=$2 AND version_id=$3`, fx.wsID, artifactID, version2).
		Scan(&revision2, &body2); err != nil {
		t.Fatal(err)
	}
	if revision2 != 2 || body2 != "能机洗吗？不建议，只能用羊毛程序。\n平铺晾干。" {
		t.Fatalf("adopted version = revision %d, body %q", revision2, body2)
	}

	var reviewBody struct {
		ReviewRequestID string `json:"review_request_id"`
		Status          string `json:"status"`
		VersionID       string `json:"version_id"`
	}
	reviewPost(t, h.SubmitContentReview, fx.wsID, "/api/content-reviews",
		fmt.Sprintf(`{"artifact_id":%q,"version_id":%q,"account_id":%q,"channel":"xiaohongshu"}`,
			artifactID, version2, fx.xhs)).Want(http.StatusCreated).JSON(&reviewBody)
	if reviewBody.ReviewRequestID == "" || reviewBody.Status != "pending" || reviewBody.VersionID != version2 {
		t.Fatalf("submitted review = %+v", reviewBody)
	}
	decide(t, fx.wsID, reviewBody.ReviewRequestID, "approved", "第 2 版人工审核通过")
	deliveryID := createDelivery(t, fx.wsID, artifactID, reviewBody.ReviewRequestID)
	advance(t, fx.wsID, deliveryID, `{"status":"ready"}`, http.StatusOK)
	advance(t, fx.wsID, deliveryID, `{"status":"handed_off","handoff_method":"export"}`, http.StatusOK)

	publicationID, publication := roiCreated(t, reviewPost(t, h.RecordContentPublication, fx.wsID,
		"/api/content-publications", fmt.Sprintf(`{"artifact_id":%q,"delivery_task_id":%q,
		"channel":"xiaohongshu","status":"reported_published","declared_by":"运营人员",
		"page_url_or_content_id":"https://example.invalid/search-lifecycle","version_match":"matched"}`,
			artifactID, deliveryID)), "publication_record_id")
	if publication["version_id"] != "" || publication["work_id"] != workID {
		t.Fatalf("publication = %v, want work %s and no redundant direct version pointer", publication, workID)
	}
	resolvedWork, resolvedArtifact, resolvedVersion, err := (feedbackPublications{db: h.DB}).Resolve(
		t.Context(), fx.wsID, publicationID)
	if err != nil || resolvedWork != workID || resolvedArtifact != artifactID || resolvedVersion != version2 {
		t.Fatalf("publication resolver = (%q, %q, %q, %v), want (%q, %q, %q)",
			resolvedWork, resolvedArtifact, resolvedVersion, err, workID, artifactID, version2)
	}

	_, metric := roiCreated(t, roiCall(t, h.RecordContentSearchMetric, fx.wsID, http.MethodPost,
		"/api/content-search/metrics", searchMetricBody(publicationID, "1240", "")), "search_metric_id")
	if metric["publication_record_id"] != publicationID || metric["metric"] != "search_impression" ||
		metric["value"] != float64(1240) || metric["data_origin"] != "manual_only" {
		t.Fatalf("search metric = %v", metric)
	}
	_, observation := roiCreated(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, http.MethodPost,
		"/api/content-search/rank-observations", strings.NewReplacer(
			`"theme_id":""`, fmt.Sprintf(`"theme_id":%q`, themeID),
			`"publication_record_id":""`, fmt.Sprintf(`"publication_record_id":%q`, publicationID),
			`"account_id":""`, fmt.Sprintf(`"account_id":%q`, fx.xhs),
		).Replace(rankObservationBody("", time.Now().UTC().Add(-time.Hour).Format(time.RFC3339), ""))), "observation_id")
	if observation["theme_id"] != themeID || observation["publication_record_id"] != publicationID ||
		observation["account_id"] != fx.xhs || observation["rule"] != "rank.single_observation" ||
		observation["data_origin"] != "manual_only" || observation["result_kind"] != "position" ||
		observation["position"] != float64(7) {
		t.Fatalf("rank observation = %v", observation)
	}

	var observations struct {
		Observations []map[string]any `json:"observations"`
	}
	roiCall(t, h.ListContentSearchRankObservations, fx.wsID, http.MethodGet,
		"/api/content-search/rank-observations?theme_id="+themeID, "").Want(http.StatusOK).JSON(&observations)
	if len(observations.Observations) != 1 || observations.Observations[0]["observation_id"] != observation["observation_id"] {
		t.Fatalf("observations by theme = %v", observations.Observations)
	}
	var metrics struct {
		Metrics []map[string]any `json:"metrics"`
	}
	roiCall(t, h.ListContentSearchMetrics, fx.wsID, http.MethodGet,
		"/api/content-search/metrics?publication_record_id="+publicationID, "").Want(http.StatusOK).JSON(&metrics)
	if len(metrics.Metrics) != 1 || metrics.Metrics[0]["search_metric_id"] != metric["search_metric_id"] ||
		metrics.Metrics[0]["publication_record_id"] != publicationID || metrics.Metrics[0]["value"] != float64(1240) {
		t.Fatalf("metrics by publication = %v", metrics.Metrics)
	}
	outsiderRequest := testutil.WithHeaders(testutil.JSONRequest(http.MethodGet,
		"/api/content-search/metrics?publication_record_id="+publicationID, nil),
		"X-User-ID", "not-a-member", "X-Workspace-ID", fx.wsID)
	testutil.Call(t, h.ListContentSearchMetrics, outsiderRequest).Want(http.StatusNotFound)
	var reviewVersion, reviewStatus string
	if err := testPool.QueryRow(t.Context(), `SELECT version_id, status FROM content_review_request WHERE review_request_id=$1`,
		reviewBody.ReviewRequestID).Scan(&reviewVersion, &reviewStatus); err != nil {
		t.Fatal(err)
	}
	if reviewVersion != version2 || reviewStatus != "approved" {
		t.Fatalf("approved review is (%s, %s), want adopted version %s approved", reviewVersion, reviewStatus, version2)
	}
}

// T089 / contract §7.1 / FR-100: a non-member and an observation that is not
// here - missing, or another brand's - answer byte for byte alike on every
// endpoint, and the path is read before the body.
func TestContentSearchObservationDecisionOrder(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := searchObservationWorkspace(t, "search-obs-order")
	other := searchObservationWorkspace(t, "search-obs-order-other")
	h := feedbackHandler(t)
	foreign, _ := roiCreated(t, roiCall(t, h.RecordContentSearchRankObservation, other.wsID, "POST", "/",
		rankObservationBody("", "2020-10-02T21:30:00+08:00", "")), "observation_id")
	reference := roiCall(t, h.ReviseContentSearchRankObservation, fx.wsID, "POST", "/",
		rankObservationBody("", "2020-10-02T21:30:00+08:00", `,"base_revision":1`), "observationId", "no-such-observation")
	reference.Want(http.StatusNotFound)

	outsider := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Search obs outsider", "slug": fmt.Sprintf("search-obs-outsider-%d", time.Now().UnixNano()), "description": "x",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id::text = $1`, outsider)
	})
	for name, response := range map[string]*testutil.Response{
		"list metrics":       roiCall(t, h.ListContentSearchMetrics, outsider, "GET", "/?publication_record_id="+fx.publication, ""),
		"record metric":      roiCall(t, h.RecordContentSearchMetric, outsider, "POST", "/", searchMetricBody(fx.publication, "1", "")),
		"list observations":  roiCall(t, h.ListContentSearchRankObservations, outsider, "GET", "/?query=x", ""),
		"record observation": roiCall(t, h.RecordContentSearchRankObservation, outsider, "POST", "/", rankObservationBody("", "2020-10-02T21:30:00+08:00", "")),
		"revise observation": roiCall(t, h.ReviseContentSearchRankObservation, outsider, "POST", "/",
			rankObservationBody("", "2020-10-02T21:30:00+08:00", `,"base_revision":1`), "observationId", foreign),
		"revise foreign": roiCall(t, h.ReviseContentSearchRankObservation, fx.wsID, "POST", "/",
			rankObservationBody("", "2020-10-02T21:30:00+08:00", `,"base_revision":1`), "observationId", foreign),
		"revise missing, bad body": roiCall(t, h.ReviseContentSearchRankObservation, fx.wsID, "POST", "/",
			`{"rank":1}`, "observationId", "no-such-observation"),
	} {
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing observation: %s", name, response.Text())
		}
	}
	if rows := searchObservationRows(t, "content_search_rank_observation_revision", other.wsID); rows != 1 {
		t.Fatalf("the foreign observation has %d rows, want its 1", rows)
	}
}

// T090 / FR-103 / FR-104 / SC-012 with the real fence: after the workspace
// deletion has committed, each adapter - searchThemes, the account check,
// the publication record check - answers ErrNotFound, never ErrStorage; the
// endpoints answer 404, not 503; nothing is left behind.
func TestContentSearchObservationAdaptersAndWritesAfterWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	fx := searchObservationWorkspace(t, "search-obs-fence")
	h := feedbackHandler(t)
	themeID, _ := roiCreated(t, roiCall(t, h.CreateContentSearchTheme, fx.wsID, "POST", "/", searchThemeBody(fx.searchFixture, "")), "theme_id")
	observationID, _ := roiCreated(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/",
		rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", "")), "observation_id")
	roiCreated(t, roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/", searchMetricBody(fx.publication, "1", "")), "search_metric_id")

	request := newRequest(http.MethodDelete, "/api/workspaces/"+fx.wsID, nil)
	request = withURLParam(request, "id", fx.wsID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)

	store := h.feedbackSearchStore()
	if err := store.Themes.ThemeExists(ctx, fx.wsID, testUserID, themeID); !errors.Is(err, feedbacklearning.ErrNotFound) || errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("theme after the deletion = %v, want ErrNotFound", err)
	}
	if err := store.Accounts.AccountExists(ctx, fx.wsID, fx.xhs); !errors.Is(err, feedbacklearning.ErrNotFound) || errors.Is(err, feedbacklearning.ErrStorage) {
		t.Errorf("account after the deletion = %v, want ErrNotFound", err)
	}
	if _, _, _, err := store.Publications.Resolve(ctx, fx.wsID, fx.publication); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Errorf("publication record after the deletion = %v, want ErrNotFound", err)
	}

	withRefs := strings.NewReplacer(`"publication_record_id":""`, fmt.Sprintf(`"publication_record_id":%q`, fx.publication),
		`"account_id":""`, fmt.Sprintf(`"account_id":%q`, fx.xhs)).Replace(rankObservationBody(themeID, "2020-10-02T21:30:00+08:00", ""))
	for name, response := range map[string]*testutil.Response{
		"record metric":      roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/", searchMetricBody(fx.publication, "1", "")),
		"record observation": roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/", withRefs),
		"revise observation": roiCall(t, h.ReviseContentSearchRankObservation, fx.wsID, "POST", "/",
			strings.Replace(withRefs, `"evidence_note"`, `"base_revision":1,"voided":true,"evidence_note"`, 1), "observationId", observationID),
		"list metrics":      roiCall(t, h.ListContentSearchMetrics, fx.wsID, "GET", "/?publication_record_id="+fx.publication, ""),
		"list observations": roiCall(t, h.ListContentSearchRankObservations, fx.wsID, "GET", "/?theme_id="+themeID, ""),
	} {
		if response.Code != http.StatusNotFound {
			t.Errorf("%s after the deletion = %d, want 404: %s", name, response.Code, response.Text())
		}
	}
	// The store itself, past the membership check: the fence refuses.
	if _, err := store.RecordRankObservation(ctx, fx.wsID, testUserID, feedbacklearning.RankObservationInput{
		Platform: "xiaohongshu", Query: "羊绒", ObservedAt: "2020-10-02T13:30:00Z", Conditions: "c",
		ResultKind: "not_found", ScannedDepth: func() *int { depth := 30; return &depth }(), EvidenceNote: "e",
	}); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Errorf("a store write after the deletion = %v, want ErrNotFound", err)
	}
	for _, table := range []string{"content_search_metric", "content_search_rank_observation_revision"} {
		if rows := searchObservationRows(t, table, fx.wsID); rows != 0 {
			t.Errorf("%s kept %d rows of the deleted workspace", table, rows)
		}
	}
}

// FR-105 / SC-012: deleting a workspace removes its search metrics and
// observations and none of a neighbour's.
func TestDeleteWorkspaceRemovesSearchMetricsAndObservations(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	target := searchObservationWorkspace(t, "search-obs-delete-target")
	neighbor := searchObservationWorkspace(t, "search-obs-delete-neighbor")
	h := feedbackHandler(t)
	for _, fx := range []searchObservationFixture{target, neighbor} {
		roiCreated(t, roiCall(t, h.RecordContentSearchMetric, fx.wsID, "POST", "/", searchMetricBody(fx.publication, "1", "")), "search_metric_id")
		roiCreated(t, roiCall(t, h.RecordContentSearchRankObservation, fx.wsID, "POST", "/",
			rankObservationBody("", "2020-10-02T21:30:00+08:00", "")), "observation_id")
	}
	request := newRequest(http.MethodDelete, "/api/workspaces/"+target.wsID, nil)
	request = withURLParam(request, "id", target.wsID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)
	for _, table := range []string{"content_search_metric", "content_search_rank_observation_revision"} {
		if rows := searchObservationRows(t, table, target.wsID); rows != 0 {
			t.Errorf("the deleted workspace kept %d %s rows", rows, table)
		}
		if rows := searchObservationRows(t, table, neighbor.wsID); rows != 1 {
			t.Errorf("the neighbour has %d %s rows, want 1", rows, table)
		}
	}
}
