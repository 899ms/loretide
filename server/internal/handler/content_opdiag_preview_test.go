package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/035 PR 2 against the real schema: the preview endpoint (T051) and
// stored versions with every dimension recomputing from the database (T053,
// SC-005). The database cases run in the handler suite
// (scripts/test-go-db.sh --suite handler); the decode case needs none.
//
// Contract: specs/035-brand-diagnosis/contracts/brand-diagnosis.md §5, §7

// opdiagAllSix selects the six dimensions with their own params.
const opdiagAllSix = `[{"key":"consistency","items":["positioning"]},{"key":"coverage","pillars":["面料知识","穿搭"]},
	{"key":"cadence"},{"key":"performance","metrics":["favorite","like"]},{"key":"audience_feedback"},
	{"key":"execution_flow"}]`

func opdiagSixParams(kind string, accountIDs []string) string {
	return strings.Replace(opdiagParams(kind, accountIDs, `,"comparison_window":{"start":"2026-08-01","end":"2026-08-31"}`),
		`"dimensions":[]`, `"dimensions":`+opdiagAllSix, 1)
}

// opdiagRecords adds a nil and a 0 metric, an excerpt and a pillar mark to
// the fixture's publication record.
func opdiagRecords(t *testing.T, fx opdiagFixture) {
	t.Helper()
	dbfx.Exec(t, `INSERT INTO content_manual_metric (manual_metric_id, workspace_id, publication_record_id,
		platform, account_id, metric, value, stat_window, sampled_at, recorded_by, source_type)
		VALUES ($1, $2, $3, 'xiaohongshu', $4, 'favorite', NULL, '发布后 7 天', '2026-09-17T02:00:00Z', 'u', 'manual'),
		       ($5, $2, $3, 'xiaohongshu', $4, 'like', 0, '发布后 7 天', '2026-09-17T02:00:00Z', 'u', 'manual')`,
		"metric-nil-"+fx.wsID, fx.wsID, fx.publicationID, fx.a1, "metric-zero-"+fx.wsID)
	dbfx.Exec(t, `INSERT INTO content_feedback_excerpt (feedback_excerpt_id, workspace_id, publication_record_id,
		source_type, redacted_excerpt, tags, occurred_at, recorded_by)
		VALUES ($1, $2, $3, 'comment', '价格有点贵', ARRAY[' 价格 ']::text[], '2026-09-11T02:00:00Z', 'u')`,
		"excerpt-"+fx.wsID, fx.wsID, fx.publicationID)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM content_feedback_excerpt WHERE workspace_id = $1`, fx.wsID)
	})
}

// The preview body takes params and nothing else; an unknown member is
// named, as on every other diagnosis body.
func TestContentOpDiagPreviewBodyNamesOnlyTheJSONField(t *testing.T) {
	for _, tc := range []struct{ body, field string }{
		{`{"title":"x"}`, "title"},
		{`{"params":{"role":"运营"}}`, "role"},
		{`{"params":{"dimensions":[{"key":"cadence","score":1}]}}`, "score"},
		{`{"params":{"roi_report_ref":{"report_id":1}}}`, "params.roi_report_ref.report_id"},
	} {
		var body opdiagPreviewBody
		err := decodeOpdiagBody(httptest.NewRecorder(), testutil.JSONRequest("POST", "/", tc.body), &body)
		fieldErr, ok := errors.AsType[feedbacklearning.FieldError](err)
		if !ok || fieldErr.Field != tc.field {
			t.Fatalf("%s: err = %v, want a FieldError naming %q", tc.body, err, tc.field)
		}
	}
}

// T051: a preview answers the full result for the params - the six
// dimensions here - and writes nothing: every content_opdiag_ table, and
// the audit log, has the same rows before and after. A non-member, a
// foreign account and a missing ROI report version are refused exactly like
// a missing record.
func TestContentOpDiagPreviewStoresNothing(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := opdiagWorkspace(t, "opdiag-preview")
	other := opdiagWorkspace(t, "opdiag-preview-other")
	opdiagRecords(t, fx)
	h := feedbackHandler(t)
	rows := func() string {
		counts := []int{}
		for _, table := range append(append([]string{}, opdiagTableNames...), "content_operation_audit") {
			counts = append(counts, opdiagRows(t, table, fx.wsID))
		}
		return fmt.Sprint(counts)
	}
	before := rows()
	var result feedbacklearning.DiagnosisResult
	roiCall(t, h.PreviewContentOpDiagReport, fx.wsID, "POST", "/api/content-operating-diagnosis/preview",
		`{"params":`+opdiagSixParams("brand", []string{fx.a1, fx.a2})+`}`).Want(http.StatusOK).JSON(&result)
	if after := rows(); after != before {
		t.Fatalf("a preview wrote rows: %s -> %s", before, after)
	}
	if len(result.Sections) < 3 || len(result.Sections[0].Dimensions) != 6 || result.Scope.Window.Timezone != "Asia/Shanghai" {
		t.Fatalf("preview = %+v", result)
	}
	var performance struct {
		Groups []struct {
			Metric  string `json:"metric"`
			Current struct {
				WithValue int     `json:"with_value"`
				Unknown   int     `json:"unknown"`
				Sum       *string `json:"sum"`
			} `json:"current"`
		} `json:"groups"`
	}
	for _, section := range result.Sections {
		if section.AccountID == fx.a1 {
			if err := json.Unmarshal(section.Dimensions[feedbacklearning.DimensionPerformance].Facts, &performance); err != nil {
				t.Fatal(err)
			}
		}
	}
	// favorite was recorded as unknown and like as 0: two groups, one of
	// each, and neither turned into the other.
	if len(performance.Groups) != 2 || performance.Groups[0].Metric != "like" || performance.Groups[0].Current.Sum == nil ||
		*performance.Groups[0].Current.Sum != "0" || performance.Groups[1].Current.Unknown != 1 || performance.Groups[1].Current.Sum != nil {
		t.Fatalf("performance = %+v", performance)
	}

	reference := roiCall(t, h.GetContentOpDiagReportVersion, fx.wsID, "GET", "/", "", "reportId", "no-such-report", "versionNo", "1")
	reference.Want(http.StatusNotFound)
	outsider := dbfx.Insert(t, "workspace", testutil.Cols{
		"name": "Opdiag preview outsider", "slug": fmt.Sprintf("opdiag-preview-outsider-%d", time.Now().UnixNano()), "description": "x",
	})
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id::text = $1`, outsider)
	})
	built := 0
	counting := func() *feedbacklearning.DiagnosisStore { built++; return h.opdiagStore() }
	response := roiCall(t, func(w http.ResponseWriter, r *http.Request) { h.previewOpdiag(w, r, counting) }, outsider,
		"POST", "/", `{"params":`+opdiagSixParams("account", []string{fx.a1})+`}`)
	response.Want(http.StatusNotFound)
	if built != 0 || !sameRefusalApartFromTrace(t, response, reference) {
		t.Fatalf("non-member: built %d, %s", built, response.Text())
	}
	for name, params := range map[string]string{
		"foreign account":     opdiagSixParams("brand", []string{fx.a1, other.a1}),
		"missing ROI version": strings.Replace(opdiagSixParams("account", []string{fx.a1}), `"dimensions":`, `"roi_report_ref":{"report_id":"no-such-roi","version_no":1},"dimensions":`, 1),
	} {
		response := roiCall(t, h.PreviewContentOpDiagReport, fx.wsID, "POST", "/", `{"params":`+params+`}`)
		response.Want(http.StatusNotFound)
		if !sameRefusalApartFromTrace(t, response, reference) {
			t.Errorf("%s answered differently from a missing record: %s", name, response.Text())
		}
	}
	assertROIField(t, roiCall(t, h.PreviewContentOpDiagReport, fx.wsID, "POST", "/",
		`{"params":`+strings.Replace(opdiagSixParams("account", []string{fx.a1}), `"items":["positioning"]`, `"items":["persona_prompt"]`, 1)+`}`),
		http.StatusBadRequest, "dimensions.consistency.items")
	if after := rows(); after != before {
		t.Fatalf("refusals wrote rows: %s -> %s", before, after)
	}
}

// T053 / SC-005 / FR-045: a version generated with all six dimensions,
// read back from the database, recomputes to its stored result under its
// own calc version - for an account report and a brand summary.
func TestContentOpDiagDimensionVersionsRecomputeFromTheDatabase(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fx := opdiagWorkspace(t, "opdiag-dimensions-recompute")
	opdiagRecords(t, fx)
	h := feedbackHandler(t)
	roiCreated(t, roiCall(t, h.RecordContentOpDiagWorkMark, fx.wsID, "POST", "/", opdiagMark(fx, "")), "mark_id")
	for _, params := range []string{
		opdiagSixParams("account", []string{fx.a1}),
		opdiagSixParams("brand", []string{fx.a1, fx.a2}),
	} {
		reportID, _ := roiCreated(t, roiCall(t, h.CreateContentOpDiagReport, fx.wsID, "POST", "/",
			opdiagRequest("six", params)), "report_id")
		stored := opdiagReportRow(t, reportID, 1)
		if !strings.Contains(stored.result, `"performance"`) || !strings.Contains(stored.result, `"execution_flow"`) {
			t.Fatalf("the stored result has no dimensions: %s", stored.result)
		}
		recomputed, err := feedbacklearning.RecomputeDiagnosis(stored.calcVersion, []byte(stored.params), []byte(stored.inputs))
		if err != nil {
			t.Fatalf("recompute: %v", err)
		}
		encoded, _ := json.Marshal(recomputed)
		if roiCanonical(t, encoded) != roiCanonical(t, []byte(stored.result)) {
			t.Fatalf("recomputed:\n%s\nstored:\n%s", encoded, stored.result)
		}
	}
}
