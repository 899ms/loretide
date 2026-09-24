package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	feedbacklearning "github.com/multica-ai/multica/server/internal/content/feedback-learning"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// specs/034 PR 3 against the real schema: importing pasted rows with an
// Idempotency-Key, the per-row duplicate trail, and the claim that belongs to
// this module (contract §1.8, §1.9). These run in the handler database suite
// (scripts/test-go-db.sh --suite handler).

func importCall(t *testing.T, h *Handler, wsID, key, body string) *testutil.Response {
	t.Helper()
	req := testutil.WithHeaders(testutil.JSONRequest("POST", "/api/content-roi/imports", body),
		"X-User-ID", testUserID, "X-Workspace-ID", wsID)
	if key != "" {
		req = testutil.WithHeaders(req, "Idempotency-Key", key)
	}
	return testutil.Call(t, h.ImportContentROI, req)
}

func importCostRow(category, amount, currency string) string {
	return fmt.Sprintf(`{"category":%q,"pricing":"amount","amount":%q,"currency":%q,
		"incurred_at":"2026-09-10T02:00:00Z"}`, category, amount, currency)
}

func importDealRow(orderRef, amount, extra string) string {
	return fmt.Sprintf(`{"order_ref":%q,"amount":%q,"currency":"CNY",
		"closed_at":"2026-09-12T02:00:00Z","gross_basis":"none"%s}`, orderRef, amount, extra)
}

func importBody(kind string, rows ...string) string {
	return fmt.Sprintf(`{"record_kind":%q,"rows":[%s]}`, kind, strings.Join(rows, ","))
}

type importRowOutcome struct {
	Row             int      `json:"row"`
	Outcome         string   `json:"outcome"`
	RecordID        string   `json:"record_id"`
	DuplicateOf     []string `json:"duplicate_of"`
	DuplicateOfRows []int    `json:"duplicate_of_rows"`
}

type importAnswer struct {
	DryRun        bool               `json:"dry_run"`
	ImportBatchID string             `json:"import_batch_id"`
	RecordKind    string             `json:"record_kind"`
	RowCount      int                `json:"row_count"`
	WrittenCount  int                `json:"written_count"`
	SkippedCount  int                `json:"skipped_count"`
	Rows          []importRowOutcome `json:"rows"`
}

func decodeImport(t *testing.T, response *testutil.Response, status int) importAnswer {
	t.Helper()
	var answer importAnswer
	response.Want(status).JSON(&answer)
	return answer
}

func sameCounts(t *testing.T, before, after map[string]int) {
	t.Helper()
	for table, want := range before {
		if after[table] != want {
			t.Errorf("%s has %d rows, want %d", table, after[table], want)
		}
	}
}

// T061 / FR-027: one bad row refuses the whole import naming its row and
// column, and nothing is written - no record, no batch, no claim, no audit.
func TestContentROIImportWithABadRowWritesNothing(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-import-bad-row")
	h := feedbackHandler(t)
	before := roiRowCounts(t, wsID)
	response := importCall(t, h, wsID, "key-bad-row", importBody("cost",
		importCostRow("拍摄", "3000.00", "CNY"), importCostRow("剪辑", "800.00", "RMB"), importCostRow("投放", "500.00", "CNY")))
	assertROIField(t, response, http.StatusBadRequest, "currency")
	if row := response.Map()["row"]; row != float64(2) {
		t.Fatalf("refusal named row %v, want 2: %s", row, response.Text())
	}
	sameCounts(t, before, roiRowCounts(t, wsID))

	// A wrong JSON type in a row names the row and the JSON field alone.
	typed := importCall(t, h, wsID, "", importBody("deal",
		importDealRow("TB-1", "1.00", ""), strings.Replace(importDealRow("TB-2", "1.00", ""), `"amount":"1.00"`, `"amount":1`, 1)))
	assertROIField(t, typed, http.StatusBadRequest, "amount")
	if row := typed.Map()["row"]; row != float64(2) {
		t.Fatalf("type refusal named row %v, want 2", row)
	}
	sameCounts(t, before, roiRowCounts(t, wsID))
}

// T062 / T063 / FR-026 / FR-029: rows matching current records are held
// back and named, with what they match; the rest are written as imports of
// this batch; a second row matching the first row of the same paste is held
// back against the record the first one wrote; a row that confirms its match
// is written and the confirmation audited. One batch, one audit event.
func TestContentROIImportMarksDuplicatesRowByRow(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-import-duplicates")
	h := feedbackHandler(t)
	existingA, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-A", "100.00", "none", "")), "deal_id")
	existingB, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-B", "200.00", "none", "")), "deal_id")
	existingC, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-C", "300.00", "none", "")), "deal_id")

	answer := decodeImport(t, importCall(t, h, wsID, "", importBody("deal",
		importDealRow("TB-A", "100.00", ""),                                                 // 1: matches A
		importDealRow("TB-NEW", "50.00", ""),                                                // 2: new
		importDealRow("TB-B", "999.00", ""),                                                 // 3: matches B (order ref alone decides)
		importDealRow("TB-NEW", "50.00", ""),                                                // 4: matches row 2's new record
		importDealRow("TB-C", "300.00", fmt.Sprintf(`,"not_duplicate_of":[%q]`, existingC)), // 5: confirmed
	)), http.StatusCreated)

	if answer.RowCount != 5 || answer.WrittenCount != 2 || answer.SkippedCount != 3 || answer.RecordKind != "deal" {
		t.Fatalf("counts = %+v", answer)
	}
	want := []struct {
		outcome     string
		duplicateOf []string
		written     bool
	}{
		{"duplicate", []string{existingA}, false},
		{"written", []string{}, true},
		{"duplicate", []string{existingB}, false},
		{"duplicate", []string{answer.Rows[1].RecordID}, false},
		{"confirmed_not_duplicate", []string{existingC}, true},
	}
	for i, row := range answer.Rows {
		if row.Row != i+1 || row.Outcome != want[i].outcome || strings.Join(row.DuplicateOf, ",") != strings.Join(want[i].duplicateOf, ",") {
			t.Errorf("row %d = %+v, want %s of %v", i+1, row, want[i].outcome, want[i].duplicateOf)
		}
		if (row.RecordID != "") != want[i].written {
			t.Errorf("row %d record id %q, written=%v", i+1, row.RecordID, want[i].written)
		}
	}
	// A written row is an import of this batch; a held-back row wrote nothing.
	for _, row := range answer.Rows {
		if row.RecordID == "" {
			continue
		}
		var source, batch string
		if err := testPool.QueryRow(t.Context(), `SELECT source_type, import_batch_id
			FROM content_roi_deal_revision WHERE workspace_id=$1 AND deal_id=$2`, wsID, row.RecordID).Scan(&source, &batch); err != nil {
			t.Fatal(err)
		}
		if source != "import" || batch != answer.ImportBatchID {
			t.Errorf("row %d stored source %q batch %q", row.Row, source, batch)
		}
	}
	var deals int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM content_roi_deal_revision WHERE workspace_id=$1`, wsID).Scan(&deals); err != nil {
		t.Fatal(err)
	}
	if deals != 5 {
		t.Fatalf("%d deal rows, want the 3 existing plus 2 written", deals)
	}
	if got := auditSteps(t, wsID, answer.Rows[4].RecordID, "confirm-not-duplicate"); got != 1 {
		t.Errorf("the confirmation left %d audit events, want 1", got)
	}
	if got := auditSteps(t, wsID, answer.ImportBatchID, "import-deal"); got != 1 {
		t.Errorf("the import left %d audit events, want 1", got)
	}

	// The batch reads back with the same trail.
	var batch struct {
		ImportBatchID string             `json:"import_batch_id"`
		RecordedBy    string             `json:"recorded_by"`
		Rows          []importRowOutcome `json:"rows"`
	}
	roiCall(t, h.GetContentROIImport, wsID, "GET", "/", "", "batchId", answer.ImportBatchID).Want(http.StatusOK).JSON(&batch)
	if batch.ImportBatchID != answer.ImportBatchID || batch.RecordedBy != testUserID || len(batch.Rows) != 5 ||
		batch.Rows[3].DuplicateOf[0] != answer.Rows[1].RecordID {
		t.Fatalf("batch = %+v", batch)
	}
	var list struct {
		Imports []struct {
			ImportBatchID string `json:"import_batch_id"`
		} `json:"imports"`
	}
	roiCall(t, h.ListContentROIImports, wsID, "GET", "/api/content-roi/imports", "").Want(http.StatusOK).JSON(&list)
	if len(list.Imports) != 1 || list.Imports[0].ImportBatchID != answer.ImportBatchID {
		t.Fatalf("list = %+v", list.Imports)
	}

	// Composed and decomposed spellings of one order reference are one order.
	nfc := decodeImport(t, importCall(t, h, wsID, "", importBody("deal",
		importDealRow("TB-é", "1.00", ""), importDealRow("TB-é", "1.00", ""))), http.StatusCreated)
	if nfc.Rows[1].Outcome != "duplicate" || nfc.Rows[1].DuplicateOf[0] != nfc.Rows[0].RecordID {
		t.Errorf("NFC: second spelling = %+v", nfc.Rows[1])
	}
}

// T064 / SC-007 / FR-028: the same key and input replays the original answer
// byte for byte and writes nothing; the same key with other input is a 409
// naming the header; the key is per record kind; a new key with the same rows
// finds every row a duplicate; a key over 255 bytes is refused.
func TestContentROIImportIdempotencyKey(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-import-key")
	h := feedbackHandler(t)
	body := importBody("cost", importCostRow("拍摄", "3000.00", "CNY"), importCostRow("剪辑", "800.00", "CNY"))

	first := importCall(t, h, wsID, "key-1", body)
	original := decodeImport(t, first, http.StatusCreated)
	if original.WrittenCount != 2 {
		t.Fatalf("first import = %s", first.Text())
	}
	afterFirst := roiRowCounts(t, wsID)
	if afterFirst["content_roi_import_claim"] != 1 || afterFirst["content_roi_import_batch"] != 1 {
		t.Fatalf("after the first import: %v", afterFirst)
	}

	// Same key, same rows spelled differently: the same answer, byte for byte.
	respelled := importBody("cost",
		`{ "currency":"CNY", "category":"拍摄", "amount":"3000.00", "pricing":"amount", "incurred_at":"2026-09-10T02:00:00Z" }`,
		importCostRow("剪辑", "800.00", "CNY"))
	for _, replayBody := range []string{body, respelled} {
		replay := importCall(t, h, wsID, "key-1", replayBody)
		replay.Want(http.StatusCreated)
		if replay.Text() != first.Text() {
			t.Fatalf("the replay differs from the original:\n%s\n%s", first.Text(), replay.Text())
		}
	}
	sameCounts(t, afterFirst, roiRowCounts(t, wsID))

	// Same key, different rows: 409 naming the header, nothing written.
	assertROIField(t, importCall(t, h, wsID, "key-1", importBody("cost", importCostRow("拍摄", "3000.01", "CNY"))),
		http.StatusConflict, "Idempotency-Key")
	sameCounts(t, afterFirst, roiRowCounts(t, wsID))

	// The same key for another record kind is another claim.
	leads := decodeImport(t, importCall(t, h, wsID, "key-1", importBody("lead",
		`{"customer_ref":"客户-0412","stage":"咨询","first_seen_at":"2026-09-10T02:00:00Z"}`)), http.StatusCreated)
	if leads.WrittenCount != 1 || leads.ImportBatchID == original.ImportBatchID {
		t.Fatalf("lead import under the same key = %+v", leads)
	}

	// Another key, the same rows: every row a possible duplicate, none written.
	again := decodeImport(t, importCall(t, h, wsID, "key-2", body), http.StatusCreated)
	if again.WrittenCount != 0 || again.SkippedCount != 2 {
		t.Fatalf("a new key re-imported the rows: %+v", again)
	}
	for i, row := range again.Rows {
		if row.Outcome != "duplicate" || len(row.DuplicateOf) != 1 || row.DuplicateOf[0] != original.Rows[i].RecordID {
			t.Errorf("row %d = %+v", i+1, row)
		}
	}

	// 256 bytes is one too many; the refusal names the header.
	before := roiRowCounts(t, wsID)
	assertROIField(t, importCall(t, h, wsID, strings.Repeat("k", 256), body), http.StatusBadRequest, "Idempotency-Key")
	sameCounts(t, before, roiRowCounts(t, wsID))
}

// T065: a dry run answers row by row and writes nothing, not even a claim.
func TestContentROIImportDryRunWritesNothing(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-import-dry-run")
	h := feedbackHandler(t)
	existing, _ := roiCreated(t, roiCall(t, h.CreateContentROIDeal, wsID, "POST", "/api/content-roi/deals",
		dealJSON("TB-OLD", "1.00", "none", "")), "deal_id")
	before := roiRowCounts(t, wsID)
	body := strings.Replace(importBody("deal",
		importDealRow("TB-OLD", "1.00", ""), importDealRow("TB-NEW", "1.00", ""), importDealRow("TB-NEW", "1.00", "")),
		`{"record_kind"`, `{"dry_run":true,"record_kind"`, 1)
	answer := decodeImport(t, importCall(t, h, wsID, "key-dry", body), http.StatusOK)
	if !answer.DryRun || answer.ImportBatchID != "" || answer.WrittenCount != 1 || answer.SkippedCount != 2 {
		t.Fatalf("dry run = %+v", answer)
	}
	if answer.Rows[0].Outcome != "duplicate" || answer.Rows[0].DuplicateOf[0] != existing {
		t.Errorf("row 1 = %+v", answer.Rows[0])
	}
	if answer.Rows[1].Outcome != "written" || answer.Rows[1].RecordID != "" {
		t.Errorf("row 2 = %+v", answer.Rows[1])
	}
	if answer.Rows[2].Outcome != "duplicate" || len(answer.Rows[2].DuplicateOfRows) != 1 || answer.Rows[2].DuplicateOfRows[0] != 2 {
		t.Errorf("row 3 = %+v", answer.Rows[2])
	}
	sameCounts(t, before, roiRowCounts(t, wsID))
	// The key was not claimed: the real import with it goes through.
	real := decodeImport(t, importCall(t, h, wsID, "key-dry", importBody("deal", importDealRow("TB-NEW", "1.00", ""))), http.StatusCreated)
	if real.WrittenCount != 1 {
		t.Fatalf("real import after the dry run = %+v", real)
	}
}

// waitForClaimWaiter blocks until another backend is waiting on a lock while
// inserting an import claim - the second importer parked on the unique index.
func waitForClaimWaiter(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND query ILIKE '%INSERT INTO content_roi_import_claim%'`).Scan(&waiting); err != nil {
			t.Error(err)
			return
		}
		if waiting > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the second importer never waited on the claim; the two did not overlap")
}

// holdFirstClaim parks the first importer right after its claim until the
// second is waiting on it, then lets the first finish.
func holdFirstClaim(t *testing.T, store *feedbacklearning.ROIStore) (claimed <-chan struct{}) {
	t.Helper()
	signal := make(chan struct{})
	var once sync.Once
	store.AfterImportClaim = func(context.Context) {
		first := false
		once.Do(func() { first = true })
		if !first {
			return
		}
		close(signal)
		waitForClaimWaiter(t)
	}
	return signal
}

func costImport(amounts ...string) feedbacklearning.ImportInput {
	in := feedbacklearning.ImportInput{RecordKind: "cost"}
	for i, amount := range amounts {
		currency := "CNY"
		if amount == "RMB" {
			amount, currency = "1.00", "RMB"
		}
		in.Costs = append(in.Costs, feedbacklearning.CostInput{
			Category: fmt.Sprintf("并发-%d", i), Pricing: "amount", Amount: &amount, Currency: currency,
			IncurredAt: "2026-09-10T02:00:00Z",
		})
	}
	return in
}

// T066 / FR-028, real database, part 1: two imports with the same key and
// input at once. The second waits on the claim's unique index while the
// first is still open, then answers with the first one's response, byte for
// byte; exactly one wrote.
func TestContentROIConcurrentImportsWithOneKeyWriteOnce(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-import-race")
	h := feedbackHandler(t)
	store := h.roiStore()
	claimed := holdFirstClaim(t, store)
	in := costImport("100.00", "200.00")

	results := make([]feedbacklearning.ImportResult, 2)
	errs := make([]error, 2)
	var done sync.WaitGroup
	done.Add(2)
	go func() {
		defer done.Done()
		results[0], errs[0] = store.Import(context.Background(), wsID, testUserID, in, "race-key", false)
	}()
	select {
	case <-claimed:
	case <-time.After(10 * time.Second):
		t.Fatal("the first import never claimed the key")
	}
	go func() {
		defer done.Done()
		results[1], errs[1] = store.Import(context.Background(), wsID, testUserID, in, "race-key", false)
	}()
	done.Wait()
	if errs[0] != nil || errs[1] != nil {
		t.Fatalf("errors: %v", errs)
	}
	first, _ := json.Marshal(results[0])
	second, _ := json.Marshal(results[1])
	if string(first) != string(second) {
		t.Fatalf("the waiting import answered differently:\n%s\n%s", first, second)
	}
	counts := roiRowCounts(t, wsID)
	if counts["content_roi_cost_revision"] != 2 || counts["content_roi_import_batch"] != 1 || counts["content_roi_import_claim"] != 1 {
		t.Fatalf("rows after two imports with one key: %v", counts)
	}
}

// T066, part 2: the first import claims the key and then fails on a bad row.
// Its claim goes with its rollback: the second import with the same key and
// corrected rows - waiting on that claim meanwhile - writes normally instead
// of meeting a 409 for "different input".
func TestContentROIImportClaimIsRolledBackWithItsImport(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-import-rollback")
	h := feedbackHandler(t)
	store := h.roiStore()
	claimed := holdFirstClaim(t, store)

	var badErr, goodErr error
	var good feedbacklearning.ImportResult
	var done sync.WaitGroup
	done.Add(2)
	go func() {
		defer done.Done()
		_, badErr = store.Import(context.Background(), wsID, testUserID, costImport("100.00", "RMB"), "retry-key", false)
	}()
	select {
	case <-claimed:
	case <-time.After(10 * time.Second):
		t.Fatal("the first import never claimed the key")
	}
	go func() {
		defer done.Done()
		good, goodErr = store.Import(context.Background(), wsID, testUserID, costImport("100.00", "300.00"), "retry-key", false)
	}()
	done.Wait()

	fieldErr, ok := errors.AsType[feedbacklearning.FieldError](badErr)
	if !ok || fieldErr.Field != "currency" || fieldErr.Row != 2 {
		t.Fatalf("the bad import answered %v, want row 2 currency", badErr)
	}
	if goodErr != nil || good.WrittenCount != 2 {
		t.Fatalf("the corrected import after the rollback = %+v, %v", good, goodErr)
	}
	counts := roiRowCounts(t, wsID)
	if counts["content_roi_cost_revision"] != 2 || counts["content_roi_import_batch"] != 1 || counts["content_roi_import_claim"] != 1 {
		t.Fatalf("rows after a rolled-back and a corrected import: %v", counts)
	}
	var claimBatch string
	if err := testPool.QueryRow(t.Context(), `SELECT import_batch_id FROM content_roi_import_claim
		WHERE workspace_id=$1 AND idempotency_key='retry-key'`, wsID).Scan(&claimBatch); err != nil {
		t.Fatal(err)
	}
	if claimBatch != good.ImportBatchID {
		t.Errorf("the claim names batch %s, the import wrote %s", claimBatch, good.ImportBatchID)
	}

	// And sequentially: a failed import leaves its key free for the retry.
	assertROIField(t, importCall(t, h, wsID, "retry-key-2", importBody("cost", importCostRow("拍摄", "1.00", "RMB"))),
		http.StatusBadRequest, "currency")
	decodeImport(t, importCall(t, h, wsID, "retry-key-2", importBody("cost", importCostRow("拍摄", "1.00", "CNY"))), http.StatusCreated)
}

// SC-008: another workspace's batch answers exactly like one that does not
// exist.
func TestContentROIImportBatchOfAnotherWorkspaceIsMissing(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	wsID, _, _ := roiWorkspace(t, "roi-import-scope")
	other, _, _ := roiWorkspace(t, "roi-import-scope-other")
	h := feedbackHandler(t)
	theirs := decodeImport(t, importCall(t, h, other, "", importBody("cost", importCostRow("拍摄", "1.00", "CNY"))), http.StatusCreated)
	foreign := roiCall(t, h.GetContentROIImport, wsID, "GET", "/", "", "batchId", theirs.ImportBatchID)
	foreign.Want(http.StatusNotFound)
	missing := roiCall(t, h.GetContentROIImport, wsID, "GET", "/", "", "batchId", "no-such-batch")
	missing.Want(http.StatusNotFound)
	if !sameRefusalApartFromTrace(t, foreign, missing) {
		t.Error("another workspace's batch and a missing one answered differently")
	}
	var list struct {
		Imports []any `json:"imports"`
	}
	roiCall(t, h.ListContentROIImports, wsID, "GET", "/api/content-roi/imports", "").Want(http.StatusOK).JSON(&list)
	if len(list.Imports) != 0 {
		t.Fatalf("the list shows another workspace's imports: %v", list.Imports)
	}
}
