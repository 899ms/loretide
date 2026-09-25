package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/035 PR 1 against the real schema: operating diagnosis report
// versions and work marks (T017 to T022, T025 to T027; SC-004, SC-005 for
// scope-only versions, SC-009, SC-012; FR-040 to FR-046, FR-080, FR-081,
// FR-084). The database cases run in the handler suite
// (scripts/test-go-db.sh --suite handler); the decode case needs none.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md

var opdiagTableNames = []string{"content_opdiag_report_version", "content_opdiag_work_mark"}

// opdiagFixture is one brand the test user owns: account a1 (xiaohongshu,
// with a profile revision) and a2 (douyin, none), a topic card for a1, a
// work from it, and one publication record of that work published in
// September 2026.
type opdiagFixture struct {
	wsID, a1, a2, revisionID, cardID, workID, publicationID string
}

func opdiagWorkspace(t *testing.T, slug string) opdiagFixture {
	t.Helper()
	slug = fmt.Sprintf("%s-%d", slug, time.Now().UnixNano())
	fx := opdiagFixture{
		a1: "acct-a1-" + slug, a2: "acct-a2-" + slug, revisionID: "rev-" + slug,
		cardID: "card-" + slug, workID: "work-" + slug, publicationID: "pub-" + slug,
	}
	fx.wsID = dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Opdiag " + slug, "slug": slug, "description": "operating diagnosis test",
	})
	dbfx.Exec(t, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, fx.wsID, testUserID)
	dbfx.Exec(t, `INSERT INTO content_account (account_id, workspace_id, platform, display_name)
		VALUES ($1, $2, 'xiaohongshu', '品牌主号'), ($3, $2, 'douyin', '副号')`, fx.a1, fx.wsID, fx.a2)
	dbfx.Exec(t, `INSERT INTO content_account_revision (revision_id, account_id, workspace_id, revision, persona_prompt)
		VALUES ($1, $2, $3, 1, '')`, fx.revisionID, fx.a1, fx.wsID)
	dbfx.Exec(t, `INSERT INTO content_topic_card (topic_card_id, workspace_id, account_id, audience_problem_judgment,
		ip_fit, timing, existing_content_relation, evidence_gaps_and_investment, channels, recommended_action)
		VALUES ($1, $2, $3, '', '', '', '', '', '[]'::jsonb, '')`, fx.cardID, fx.wsID, fx.a1)
	dbfx.Exec(t, `INSERT INTO content_work (work_id, workspace_id, topic_card_id, snapshot_id, title)
		VALUES ($1, $2, $3, '', '面料对比')`, fx.workID, fx.wsID, fx.cardID)
	dbfx.Exec(t, `INSERT INTO content_publication_record
		(publication_record_id, workspace_id, work_id, artifact_id, delivery_task_id,
		 channel, status, actor_id, page_url_or_content_id, published_at)
		VALUES ($1,$2,$3,'a','','xiaohongshu','verified_published',$4,'https://example.invalid/p/1',
		 '2026-09-10T02:00:00Z')`, fx.publicationID, fx.wsID, fx.workID, testUserID)
	t.Cleanup(func() {
		background := context.Background()
		for _, statement := range []string{
			`DELETE FROM content_opdiag_report_version WHERE workspace_id = $1`,
			`DELETE FROM content_opdiag_work_mark WHERE workspace_id = $1`,
			`DELETE FROM content_manual_metric WHERE workspace_id = $1`,
			`DELETE FROM content_publication_record WHERE workspace_id = $1`,
			`DELETE FROM content_work WHERE workspace_id = $1`,
			`DELETE FROM content_topic_card WHERE workspace_id = $1`,
			`DELETE FROM content_account_revision WHERE workspace_id = $1`,
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

// opdiagParams is contract §4's params for September 2026, over the given
// scope, with extra JSON members appended.
func opdiagParams(kind string, accountIDs []string, extra string) string {
	ids, _ := json.Marshal(accountIDs)
	return fmt.Sprintf(`{"scope":{"kind":%q,"account_ids":%s},"window":{"start":"2026-09-01","end":"2026-09-30"},
		"dimensions":[]%s}`, kind, ids, extra)
}

func opdiagRequest(title, params string) string {
	return fmt.Sprintf(`{"title":%q,"params":%s}`, title, params)
}

type opdiagStoredVersion struct {
	calcVersion, params, inputs, result, row string
}

func opdiagReportRow(t *testing.T, reportID string, versionNo int) opdiagStoredVersion {
	t.Helper()
	var stored opdiagStoredVersion
	if err := testPool.QueryRow(t.Context(), `SELECT calc_version, params::text, inputs::text, result::text,
		row_to_json(r)::text FROM content_opdiag_report_version r WHERE report_id=$1 AND version_no=$2`,
		reportID, versionNo).Scan(&stored.calcVersion, &stored.params, &stored.inputs, &stored.result, &stored.row); err != nil {
		t.Fatalf("read diagnosis %s v%d: %v", reportID, versionNo, err)
	}
	return stored
}

func opdiagRows(t *testing.T, table, wsID string) int {
	t.Helper()
	var rows int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM `+table+` WHERE workspace_id=$1`, wsID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func opdiagMark(fx opdiagFixture, extra string) string {
	return fmt.Sprintf(`{"work_id":%q,"kind":"pillar","item":"面料知识","verdict":"tagged"%s}`, fx.workID, extra)
}

// ---------------------------------------------------------------- decode (no database)

// Decode errors name the JSON member only: an unknown one by its own name
// - role and industry are the cases FR-004 names - and a wrongly typed one
// by its JSON path. No Go type name reaches a response.
func TestContentOpDiagBodiesNameOnlyTheJSONField(t *testing.T) {
	cases := []struct {
		name, body, field string
		target            func() any
	}{
		{"role at the top", `{"role":"运营"}`, "role", func() any { return &opdiagReportBody{} }},
		{"industry in params", `{"params":{"industry":"服装"}}`, "industry", func() any { return &opdiagReportBody{} }},
		{"role in scope", `{"params":{"scope":{"kind":"brand","account_ids":[],"role":"x"}}}`, "role", func() any { return &opdiagReportBody{} }},
		{"level on a dimension", `{"params":{"dimensions":[{"key":"cadence","level":1}]}}`, "level", func() any { return &opdiagReportBody{} }},
		{"title", `{"title":1}`, "title", func() any { return &opdiagReportBody{} }},
		{"window start", `{"params":{"window":{"start":20260901}}}`, "params.window.start", func() any { return &opdiagReportBody{} }},
		{"account ids", `{"params":{"scope":{"account_ids":"a1"}}}`, "params.scope.account_ids", func() any { return &opdiagReportBody{} }},
		{"mark work", `{"work_id":1}`, "work_id", func() any { return &opdiagMarkBody{} }},
		{"mark score", `{"work_id":"w","score":3}`, "score", func() any { return &opdiagMarkBody{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeOpdiagBody(httptest.NewRecorder(), testutil.JSONRequest("POST", "/", tc.body), tc.target())
			fieldErr, ok := errors.AsType[feedbacklearning.FieldError](err)
			if !ok || fieldErr.Field != tc.field {
				t.Fatalf("err = %v, want a FieldError naming %q", err, tc.field)
			}
			if strings.ContainsFunc(fieldErr.Field, func(r rune) bool { return r >= 'A' && r <= 'Z' }) {
				t.Fatalf("a Go name reached the field: %q", fieldErr.Field)
			}
		})
	}
	// The fields the server writes are accepted and dropped.
	var mark opdiagMarkBody
	if err := decodeOpdiagBody(httptest.NewRecorder(), testutil.JSONRequest("POST", "/",
		`{"work_id":"w","kind":"consistency","item":"positioning","verdict":"unsure","account_id":"a1",
		  "profile_revision_id":"chosen-by-client","recorded_by":"someone","mark_id":"m","created_at":"x"}`), &mark); err != nil {
		t.Fatalf("server-written fields refused: %v", err)
	}
	var report opdiagReportBody
	if err := decodeOpdiagBody(httptest.NewRecorder(), testutil.JSONRequest("POST", "/",
		`{"params":{"window":{"start":"2026-09-01","end":"2026-09-30","timezone":"UTC"},"generated_at":"1999-01-01T00:00:00Z"}}`),
		&report); err != nil {
		t.Fatalf("timezone and generated_at refused: %v", err)
	}
}

// ---------------------------------------------------------------- versions

// T017 / T018 / SC-004 / FR-040 to FR-044: version 1 reads back exactly as
// stored; a mark added afterwards leaves every stored byte of it alone and
// reading it says - derived, not stored - which input moved on. Version 2
// takes version 1's params, and both stay.
func TestContentOpDiagReportVersionsAreNeverRewritten(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := opdiagWorkspace(t, "opdiag-versions")
	h := feedbackHandler(t)
	params := opdiagParams("brand", []string{fx.a2, fx.a1}, `,"generated_at":"1999-01-01T00:00:00Z"`)
	params = strings.Replace(params, `"end":"2026-09-30"`, `"end":"2026-09-30","timezone":"UTC"`, 1)
	reportID, first := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST",
		"/api/content-operating-diagnosis/reports", opdiagRequest("九月诊断", params)), "report_id")
	if first["version_no"] != float64(1) || first["ai_judgement_state"] != "pending_data" ||
		first["scope_kind"] != "brand" || first["calc_version"] != "opdiag-calc/1" {
		t.Fatalf("version 1 = %v", first)
	}
	v1 := opdiagReportRow(t, reportID, 1)
	var storedParams feedbacklearning.DiagnosisParams
	if err := json.Unmarshal([]byte(v1.params), &storedParams); err != nil {
		t.Fatal(err)
	}
	if storedParams.Window.Timezone != "Asia/Shanghai" || strings.HasPrefix(storedParams.GeneratedAt, "1999") {
		t.Fatalf("the server-written params were the body's: %+v", storedParams)
	}
	var result feedbacklearning.DiagnosisResult
	if err := json.Unmarshal([]byte(v1.result), &result); err != nil {
		t.Fatal(err)
	}
	if result.DataOrigin != "manual_only" || result.Scope.InputCounts.Publications != 1 ||
		len(result.Scope.Accounts) != 2 || result.Scope.Accounts[0].DisplayName != "副号" ||
		result.Scope.Accounts[1].ProfileRevisionID != fx.revisionID {
		t.Fatalf("version 1 result = %s", v1.result)
	}

	// Read back: the three columns as stored.
	var read struct {
		InputsChanged feedbacklearning.DiagnosisInputsChanged `json:"inputs_changed"`
		Params        json.RawMessage                         `json:"params"`
		Inputs        json.RawMessage                         `json:"inputs"`
		Result        json.RawMessage                         `json:"result"`
	}
	roiCall(t, h.GetContentOpDiagReportVersion, fx.wsID, "GET", "/", "", "reportId", reportID, "versionNo", "1").
		Want(http.StatusOK).JSON(&read)
	for name, pair := range map[string][2]string{
		"params": {string(read.Params), v1.params}, "inputs": {string(read.Inputs), v1.inputs},
		"result": {string(read.Result), v1.result},
	} {
		if roiCanonical(t, []byte(pair[0])) != roiCanonical(t, []byte(pair[1])) {
			t.Errorf("%s read back differently from what was stored", name)
		}
	}
	if read.InputsChanged.Changed {
		t.Fatalf("a fresh version reads as changed: %+v", read.InputsChanged)
	}

	// A mark on the work version 1 read.
	markID, _ := roiCreated(t, roiCall(t, h.RecordContentOpDiagWorkMark, fx.wsID, "POST",
		"/api/content-operating-diagnosis/work-marks", opdiagMark(fx, "")), "mark_id")
	if got := opdiagReportRow(t, reportID, 1); got != v1 {
		t.Fatalf("version 1 changed after a mark was added:\n%s\n%s", v1.row, got.row)
	}
	roiCall(t, h.GetContentOpDiagReportVersion, fx.wsID, "GET", "/", "", "reportId", reportID, "versionNo", "1").
		Want(http.StatusOK).JSON(&read)
	if !read.InputsChanged.Changed || !slices.Contains(read.InputsChanged.Added,
		feedbacklearning.DiagnosisRecordRef{Kind: "work_mark", ID: markID}) {
		t.Fatalf("inputs_changed after the mark = %+v", read.InputsChanged)
	}
	var flagColumns int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'content_opdiag_report_version'
		  AND (column_name LIKE '%changed%' OR column_name LIKE '%stale%')`).Scan(&flagColumns); err != nil {
		t.Fatal(err)
	}
	if flagColumns != 0 {
		t.Fatalf("the report table stores %d changed/stale column(s)", flagColumns)
	}

	// Version 2 with version 1's params, from the inputs as they are now.
	_, second := roiCreated(t, roiCall(t, h.GenerateContentOpDiagReportVersion, fx.wsID, "POST", "/versions", `{}`,
		"reportId", reportID), "report_id")
	if second["version_no"] != float64(2) || second["title"] != "九月诊断" {
		t.Fatalf("version 2 = %v", second)
	}
	v2 := opdiagReportRow(t, reportID, 2)
	var secondParams feedbacklearning.DiagnosisParams
	if err := json.Unmarshal([]byte(v2.params), &secondParams); err != nil {
		t.Fatal(err)
	}
	if secondParams.Window != storedParams.Window || !slices.Equal(secondParams.Scope.AccountIDs, storedParams.Scope.AccountIDs) ||
		secondParams.Scope.Kind != storedParams.Scope.Kind {
		t.Fatalf("version 2 did not reuse version 1's params:\n%+v\n%+v", storedParams, secondParams)
	}
	if !strings.Contains(v2.inputs, markID) || strings.Contains(v1.inputs, markID) {
		t.Fatal("version 2 should hold the mark and version 1 should not")
	}
	if got := opdiagReportRow(t, reportID, 1); got != v1 {
		t.Fatal("generating version 2 changed version 1")
	}
	var versions struct {
		ReportID string           `json:"report_id"`
		Versions []map[string]any `json:"versions"`
	}
	roiCall(t, h.ListContentOpDiagReportVersions, fx.wsID, "GET", "/", "", "reportId", reportID).
		Want(http.StatusOK).JSON(&versions)
	if versions.ReportID != reportID || len(versions.Versions) != 2 {
		t.Fatalf("versions = %+v", versions)
	}
	var reports struct {
		Reports []map[string]any `json:"reports"`
	}
	roiCall(t, h.ListContentOpDiagReports, fx.wsID, "GET", "/", "").Want(http.StatusOK).JSON(&reports)
	if len(reports.Reports) != 1 || reports.Reports[0]["version_no"] != float64(2) {
		t.Fatalf("reports = %+v", reports.Reports)
	}
	if _, leaked := reports.Reports[0]["inputs"]; leaked {
		t.Fatal("the report list carries inputs")
	}
}

// T019 / FR-045: every stored version, read back from the database,
// recomputes to its stored result under its own calc version.
func TestContentOpDiagStoredVersionsRecomputeFromTheDatabase(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := opdiagWorkspace(t, "opdiag-recompute")
	h := feedbackHandler(t)
	roiCreated(t, roiCall(t, h.RecordContentOpDiagWorkMark, fx.wsID, "POST", "/", opdiagMark(fx, "")), "mark_id")
	for _, params := range []string{
		opdiagParams("account", []string{fx.a1}, ""),
		opdiagParams("brand", []string{fx.a1, fx.a2}, ""),
		opdiagParams("account", []string{fx.a2}, `,"comparison_window":{"start":"2026-08-01","end":"2026-08-31"}`),
	} {
		reportID, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/",
			opdiagRequest("recompute", params)), "report_id")
		stored := opdiagReportRow(t, reportID, 1)
		recomputed, err := feedbacklearning.RecomputeDiagnosis(stored.calcVersion, []byte(stored.params), []byte(stored.inputs))
		if err != nil {
			t.Fatalf("recompute %s: %v", params, err)
		}
		encoded, _ := json.Marshal(recomputed)
		if roiCanonical(t, encoded) != roiCanonical(t, []byte(stored.result)) {
			t.Fatalf("recomputed:\n%s\nstored:\n%s", encoded, stored.result)
		}
	}
}

// T020: two generations of the same report at the same moment get versions
// 2 and 3 - the per-report lock queues the second behind the first - and
// neither fails.
func TestContentOpDiagConcurrentGenerationsGetConsecutiveVersions(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := opdiagWorkspace(t, "opdiag-concurrent")
	h := feedbackHandler(t)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/",
		opdiagRequest("concurrent", opdiagParams("account", []string{fx.a1}, ""))), "report_id")
	store := h.opdiagStore()
	var wg sync.WaitGroup
	got := make([]int, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range 2 {
		wg.Go(func() {
			<-start
			version, err := store.GenerateDiagnosisReportVersion(context.Background(), fx.wsID, testUserID, reportID,
				feedbacklearning.DiagnosisReportRequest{}, time.Now())
			got[i], errs[i] = version.VersionNo, err
		})
	}
	close(start)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("a concurrent generation failed: %v", err)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, []int{2, 3}) {
		t.Fatalf("versions = %v, want 2 and 3", got)
	}
	var rows int
	if err := testPool.QueryRow(t.Context(), `SELECT count(DISTINCT version_no) FROM content_opdiag_report_version
		WHERE report_id=$1`, reportID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("%d distinct versions stored, want 3", rows)
	}
}

// ---------------------------------------------------------------- marks

// T021: a consistency mark needs an account and takes the profile revision
// the server reads, never one the body names; a pillar takes tagged or
// untagged; the list answers the latest mark per (work, kind, item).
func TestContentOpDiagWorkMarksKeepTheirHistory(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := opdiagWorkspace(t, "opdiag-marks")
	h := feedbackHandler(t)
	mark := func(body string) *testutil.Response {
		return roiCall(t, h.RecordContentOpDiagWorkMark, fx.wsID, "POST", "/api/content-operating-diagnosis/work-marks", body)
	}
	consistency := func(account, verdict, extra string) string {
		return fmt.Sprintf(`{"work_id":%q,"kind":"consistency","item":"positioning","verdict":%q,"account_id":%q%s}`,
			fx.workID, verdict, account, extra)
	}
	assertROIField(t, mark(consistency("", "consistent", "")), http.StatusBadRequest, "account_id")
	assertROIField(t, mark(fmt.Sprintf(`{"work_id":%q,"kind":"pillar","item":"穿搭","verdict":"unsure"}`, fx.workID)),
		http.StatusBadRequest, "verdict")
	assertROIField(t, mark(consistency(fx.a1, "tagged", "")), http.StatusBadRequest, "verdict")
	// a2 has no profile revision: nothing to check against.
	assertROIField(t, mark(consistency(fx.a2, "consistent", "")), http.StatusBadRequest, "account_id")

	_, written := roiCreated(t, mark(consistency(fx.a1, "inconsistent", `,"profile_revision_id":"chosen-by-client"`)), "mark_id")
	if written["profile_revision_id"] != fx.revisionID || written["recorded_by"] != testUserID {
		t.Fatalf("mark = %v", written)
	}
	roiCreated(t, mark(fmt.Sprintf(`{"work_id":%q,"kind":"pillar","item":" 穿搭 ","verdict":"tagged"}`, fx.workID)), "mark_id")
	roiCreated(t, mark(fmt.Sprintf(`{"work_id":%q,"kind":"pillar","item":"穿搭","verdict":"untagged"}`, fx.workID)), "mark_id")

	var list struct {
		Marks []feedbacklearning.WorkMark `json:"marks"`
	}
	roiCall(t, h.ListContentOpDiagWorkMarks, fx.wsID, "GET", "/api/content-operating-diagnosis/work-marks?work_id="+fx.workID, "").
		Want(http.StatusOK).JSON(&list)
	current := map[string]feedbacklearning.MarkVerdict{}
	for _, m := range list.Marks {
		current[string(m.Kind)+"/"+m.Item] = m.Verdict
	}
	if len(list.Marks) != 2 || current["pillar/穿搭"] != "untagged" || current["consistency/positioning"] != "inconsistent" {
		t.Fatalf("current marks = %+v", list.Marks)
	}
	if rows := opdiagRows(t, "content_opdiag_work_mark", fx.wsID); rows != 3 {
		t.Fatalf("%d mark rows, want all 3 kept", rows)
	}
	roiCall(t, h.ListContentOpDiagWorkMarks, fx.wsID, "GET", "/?account_id="+fx.a1, "").Want(http.StatusOK).JSON(&list)
	if len(list.Marks) != 1 || list.Marks[0].Kind != "consistency" {
		t.Fatalf("marks for a1 = %+v", list.Marks)
	}
}

// ---------------------------------------------------------------- fence and deletion

// T022 / SC-012 / FR-084: after a workspace deletion has committed,
// generating a report, generating a version and recording a mark are each
// refused by the fence and leave no row.
func TestContentOpDiagWritesAreFencedByWorkspaceDeletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := t.Context()
	fx := opdiagWorkspace(t, "opdiag-fence")
	h := feedbackHandler(t)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/",
		opdiagRequest("fence", opdiagParams("account", []string{fx.a1}, ""))), "report_id")
	if _, err := testPool.Exec(ctx, `DELETE FROM workspace WHERE id::text = $1`, fx.wsID); err != nil {
		t.Fatal(err)
	}
	var params feedbacklearning.DiagnosisParams
	if err := json.Unmarshal([]byte(opdiagParams("account", []string{fx.a1}, "")), &params); err != nil {
		t.Fatal(err)
	}
	store := h.opdiagStore()
	if _, err := store.CreateDiagnosisReport(ctx, fx.wsID, testUserID, feedbacklearning.DiagnosisReportRequest{Params: &params}, time.Now()); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Fatalf("a new report after the delete committed = %v, want ErrNotFound", err)
	}
	if _, err := store.GenerateDiagnosisReportVersion(ctx, fx.wsID, testUserID, reportID, feedbacklearning.DiagnosisReportRequest{}, time.Now()); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Fatalf("a new version after the delete committed = %v, want ErrNotFound", err)
	}
	if _, err := store.RecordWorkMark(ctx, fx.wsID, testUserID, feedbacklearning.WorkMarkInput{
		WorkID: fx.workID, Kind: feedbacklearning.MarkPillar, Item: "穿搭", Verdict: feedbacklearning.VerdictTagged,
	}); !errors.Is(err, feedbacklearning.ErrNotFound) {
		t.Fatalf("a mark after the delete committed = %v, want ErrNotFound", err)
	}
	if rows := opdiagRows(t, "content_opdiag_report_version", fx.wsID); rows != 1 {
		t.Fatalf("%d report rows after fenced writes, want the 1 from before", rows)
	}
	if rows := opdiagRows(t, "content_opdiag_work_mark", fx.wsID); rows != 0 {
		t.Fatalf("%d mark rows after a fenced write", rows)
	}
}

// T025 / FR-085: deleting a workspace removes both tables' rows in it and
// none of a neighbour's.
func TestDeleteWorkspaceRemovesOpDiagRecords(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	target := opdiagWorkspace(t, "opdiag-delete-target")
	neighbor := opdiagWorkspace(t, "opdiag-delete-neighbor")
	h := feedbackHandler(t)
	for _, fx := range []opdiagFixture{target, neighbor} {
		roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/",
			opdiagRequest("delete", opdiagParams("account", []string{fx.a1}, ""))), "report_id")
		roiCreated(t, roiCall(t, h.RecordContentOpDiagWorkMark, fx.wsID, "POST", "/", opdiagMark(fx, "")), "mark_id")
	}
	request := newRequest(http.MethodDelete, "/api/workspaces/"+target.wsID, nil)
	request = withURLParam(request, "id", target.wsID)
	testutil.Call(t, testHandler.DeleteWorkspace, request).Want(http.StatusNoContent)
	for _, table := range opdiagTableNames {
		if rows := opdiagRows(t, table, target.wsID); rows != 0 {
			t.Errorf("%s kept %d rows of the deleted workspace", table, rows)
		}
		if rows := opdiagRows(t, table, neighbor.wsID); rows != 1 {
			t.Errorf("%s has %d neighbour rows, want 1", table, rows)
		}
	}
}

// ---------------------------------------------------------------- refusals

// T026 / T027 / FR-080 / FR-081 / SC-009 / D14-V01 / contract §7.1: a
// caller who is not a member is refused before the store is even built, on
// every endpoint; another brand's account in the params, a report that is
// not here and a version number that is not one all answer byte for byte
// like a missing record; then the params are refused by name.
func TestContentOpDiagRefusalsAnswerLikeMissingRecords(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := opdiagWorkspace(t, "opdiag-refusals")
	other := opdiagWorkspace(t, "opdiag-refusals-other")
	h := feedbackHandler(t)
	reportID, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/",
		opdiagRequest("refusals", opdiagParams("account", []string{fx.a1}, ""))), "report_id")
	foreignReport, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, other.wsID, "POST", "/",
		opdiagRequest("foreign", opdiagParams("account", []string{other.a1}, ""))), "report_id")
	reference := roiCall(t, h.GetContentOpDiagReportVersion, fx.wsID, "GET", "/", "", "reportId", "no-such-report", "versionNo", "1")
	reference.Want(http.StatusNotFound)

	// Not a member: nothing is built, nothing is read, on any endpoint.
	outsider := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Opdiag outsider", "slug": fmt.Sprintf("opdiag-outsider-%d", time.Now().UnixNano()), "description": "x",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id::text = $1`, outsider)
	})
	built := 0
	counting := func() *feedbacklearning.DiagnosisStore { built++; return h.opdiagStore() }
	body := opdiagRequest("x", opdiagParams("account", []string{fx.a1}, ""))
	for name, call := range map[string]func() *testutil.Response{
		"list reports": func() *testutil.Response {
			return roiCall(t, func(w http.ResponseWriter, r *http.Request) { h.listOpdiagReports(w, r, counting) }, outsider, "GET", "/", "")
		},
		"create report": func() *testutil.Response {
			return roiCall(t, func(w http.ResponseWriter, r *http.Request) { h.createOpdiagReport(w, r, counting) }, outsider, "POST", "/", body)
		},
		"record mark": func() *testutil.Response {
			return roiCall(t, func(w http.ResponseWriter, r *http.Request) { h.recordOpdiagWorkMark(w, r, counting) }, outsider, "POST", "/", opdiagMark(fx, ""))
		},
		"list versions": func() *testutil.Response {
			return roiCall(t, h.ListContentOpDiagReportVersions, outsider, "GET", "/", "", "reportId", reportID)
		},
		"new version": func() *testutil.Response {
			return roiCall(t, h.GenerateContentOpDiagReportVersion, outsider, "POST", "/", "{}", "reportId", reportID)
		},
		"get version": func() *testutil.Response {
			return roiCall(t, h.GetContentOpDiagReportVersion, outsider, "GET", "/", "", "reportId", reportID, "versionNo", "1")
		},
		"list marks": func() *testutil.Response { return roiCall(t, h.ListContentOpDiagWorkMarks, outsider, "GET", "/", "") },
	} {
		response := call()
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s as a non-member answered differently from a missing record: %s", name, response.Text())
		}
	}
	if built != 0 {
		t.Fatalf("the store was built %d time(s) for a non-member", built)
	}

	// Another brand's account, alone or beside one of ours, and with params
	// that are otherwise wrong too: the same refusal as a missing account.
	for name, request := range map[string]string{
		"foreign account":         opdiagRequest("x", opdiagParams("account", []string{other.a1}, "")),
		"foreign beside ours":     opdiagRequest("x", opdiagParams("brand", []string{fx.a1, other.a1}, "")),
		"missing account":         opdiagRequest("x", opdiagParams("account", []string{"no-such-account"}, "")),
		"foreign with bad window": opdiagRequest("x", strings.Replace(opdiagParams("account", []string{other.a1}, ""), `"end":"2026-09-30"`, `"end":"2026-01-01"`, 1)),
		"foreign mark account":    `{"work_id":"` + fx.workID + `","kind":"consistency","item":"positioning","verdict":"unsure","account_id":"` + other.a1 + `"}`,
		"foreign mark work":       opdiagMark(opdiagFixture{workID: other.workID}, ""),
	} {
		handler := h.CreateContentOpDiagReport
		if strings.Contains(name, "mark") {
			handler = h.RecordContentOpDiagWorkMark
		}
		response := roiCall(t, handler, fx.wsID, "POST", "/", request)
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing record: %s", name, response.Text())
		}
	}
	if rows := opdiagRows(t, "content_opdiag_report_version", fx.wsID); rows != 1 {
		t.Fatalf("%d report rows after refusals, want 1", rows)
	}

	// Paths that are not here.
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		method  string
		params  []string
	}{
		{"version abc", h.GetContentOpDiagReportVersion, "GET", []string{"reportId", reportID, "versionNo", "abc"}},
		{"version 0", h.GetContentOpDiagReportVersion, "GET", []string{"reportId", reportID, "versionNo", "0"}},
		{"version 1.0", h.GetContentOpDiagReportVersion, "GET", []string{"reportId", reportID, "versionNo", "1.0"}},
		{"version 99", h.GetContentOpDiagReportVersion, "GET", []string{"reportId", reportID, "versionNo", "99"}},
		{"foreign report", h.GetContentOpDiagReportVersion, "GET", []string{"reportId", foreignReport, "versionNo", "1"}},
		{"foreign versions", h.ListContentOpDiagReportVersions, "GET", []string{"reportId", foreignReport}},
		{"new version of a foreign report", h.GenerateContentOpDiagReportVersion, "POST", []string{"reportId", foreignReport}},
		{"new version of a missing report", h.GenerateContentOpDiagReportVersion, "POST", []string{"reportId", "no-such-report"}},
	} {
		body := ""
		if tc.method == "POST" {
			body = "{}"
		}
		response := roiCall(t, tc.handler, fx.wsID, tc.method, "/", body, tc.params...)
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing record: %s", tc.name, response.Text())
		}
	}

	// Then the caller's own params, by name.
	for _, tc := range []struct{ name, body, field string }{
		{"role", `{"role":"运营","params":` + opdiagParams("account", []string{fx.a1}, "") + `}`, "role"},
		{"industry", opdiagRequest("x", opdiagParams("account", []string{fx.a1}, `,"industry":"服装"`)), "industry"},
		{"empty brand", opdiagRequest("x", opdiagParams("brand", []string{}, "")), "scope.account_ids"},
		{"two accounts in an account report", opdiagRequest("x", opdiagParams("account", []string{fx.a1, fx.a2}, "")), "scope.account_ids"},
		{"window", opdiagRequest("x", strings.Replace(opdiagParams("account", []string{fx.a1}, ""), `"end":"2026-09-30"`, `"end":"2026-08-01"`, 1)), "window"},
		{"a dimension", opdiagRequest("x", strings.Replace(opdiagParams("account", []string{fx.a1}, ""), `"dimensions":[]`, `"dimensions":[{"key":"cadence"}]`, 1)), "dimensions"},
		{"no params", `{"title":"x"}`, "params"},
		{"title", opdiagRequest(strings.Repeat("诊", feedbacklearning.MaxDiagnosisTitleRunes+1), opdiagParams("account", []string{fx.a1}, "")), "title"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertROIField(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/", tc.body), http.StatusBadRequest, tc.field)
		})
	}
	if rows := opdiagRows(t, "content_opdiag_report_version", fx.wsID); rows != 1 {
		t.Fatalf("%d report rows after refusals, want 1", rows)
	}
}
