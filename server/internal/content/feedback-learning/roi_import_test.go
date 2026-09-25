package feedbacklearning

import (
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// specs/034 PR 3 without a database: the guards (T067) and the pure parts of
// an import. The real-database cases - concurrency, rollback, replay, the
// duplicate trail - are in internal/handler's content_roi_import_test.go and
// cmd/server's content_roi_routes_test.go, which CI and the remote runner
// execute.

// T067: idempotency is built in this module and the idempotency module is
// not imported - by any file, test files included, since a test importing it
// would be the first step of a production file doing the same.
func TestFeedbackLearningDoesNotImportTheIdempotencyModule(t *testing.T) {
	matches, err := filepath.Glob(filepath.Join(moduleDir(t), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) < 10 {
		t.Fatalf("found only %d Go files; the guard would pass vacuously", len(matches))
	}
	fset := token.NewFileSet()
	for _, path := range matches {
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			if strings.HasSuffix(imported, "/internal/content/idempotency") {
				t.Errorf("%s imports %s; the import claim is this module's own (contract §1.9)",
					filepath.Base(path), imported)
			}
		}
	}
}

// T067: the claim table is insert-only. TestROITablesHaveAnInsertAndNoUpdateOrDelete
// covers it with the other tables; this names it, and checks the claim is
// written with ON CONFLICT DO NOTHING - the statement a second request with
// the same key waits on - rather than read-then-insert.
func TestTheImportClaimIsInsertOnly(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	for _, forbidden := range []string{
		"UPDATE CONTENT_ROI_IMPORT_CLAIM", "DELETE FROM CONTENT_ROI_IMPORT_CLAIM", "TRUNCATE CONTENT_ROI_IMPORT_CLAIM",
		"UPDATE CONTENT_ROI_IMPORT_BATCH", "DELETE FROM CONTENT_ROI_IMPORT_BATCH",
	} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("found %q", forbidden)
		}
	}
	if !strings.Contains(upper, "INSERT INTO CONTENT_ROI_IMPORT_CLAIM") {
		t.Fatal("no INSERT INTO content_roi_import_claim; the guard would pass vacuously")
	}
	if !strings.Contains(upper, "ON CONFLICT (WORKSPACE_ID, RECORD_KIND, IDEMPOTENCY_KEY) DO NOTHING") {
		t.Error("the claim is not an INSERT ... ON CONFLICT DO NOTHING on the unique key")
	}
}

// The claim and the batch are written in the import's transaction: nothing in
// roi_import.go opens a second one or commits early. An independent commit of
// the claim is what would survive a rollback of the import it belongs to.
func TestTheImportOpensOneTransaction(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(moduleDir(t), "roi_import.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := stripGoComments(string(body))
	if got := strings.Count(code, "s.begin("); got != 1 {
		t.Errorf("roi_import.go begins %d transactions, want 1", got)
	}
	for _, forbidden := range []string{"s.DB.Begin(", ".Exec(ctx, `INSERT INTO content_roi_import_claim", "s.DB.Exec("} {
		if strings.Contains(code, forbidden) {
			t.Errorf("roi_import.go contains %q", forbidden)
		}
	}
	if got := strings.Count(code, ".Commit("); got != 1 {
		t.Errorf("roi_import.go commits %d times, want 1", got)
	}
}

func importCost(category, amount, currency string) CostInput {
	return CostInput{
		Category: category, Pricing: "amount", Amount: &amount, Currency: currency,
		IncurredAt: "2026-09-10T02:00:00Z",
	}
}

// FR-028: the fingerprint is over what the rows say, not how they were
// spelled: the same rows give the same fingerprint, any change gives another,
// and not_duplicate_of is part of it.
func TestTheImportFingerprintIsTheRowsNotTheirSpelling(t *testing.T) {
	base := ImportInput{RecordKind: "cost", Costs: []CostInput{importCost("拍摄", "3000.00", "CNY")}}
	first, err := importFingerprint(base)
	if err != nil || len(first) != 64 {
		t.Fatalf("fingerprint %q, %v", first, err)
	}
	again, _ := importFingerprint(ImportInput{RecordKind: "cost", Costs: []CostInput{importCost("拍摄", "3000.00", "CNY")}})
	if again != first {
		t.Error("the same rows gave two fingerprints")
	}
	// Decoded from differently spelled JSON: key order and spacing.
	var decoded CostInput
	if err = json.Unmarshal([]byte(`{ "incurred_at":"2026-09-10T02:00:00Z", "currency":"CNY",
		"amount":"3000.00", "pricing":"amount", "category":"拍摄" }`), &decoded); err != nil {
		t.Fatal(err)
	}
	if spelled, _ := importFingerprint(ImportInput{RecordKind: "cost", Costs: []CostInput{decoded}}); spelled != first {
		t.Error("key order or whitespace changed the fingerprint")
	}
	changed := []ImportInput{
		{RecordKind: "cost", Costs: []CostInput{importCost("拍摄", "3000.01", "CNY")}},
		{RecordKind: "cost", Costs: []CostInput{importCost("拍摄", "3000.00", "CNY"), importCost("拍摄", "3000.00", "CNY")}},
		{RecordKind: "lead", Leads: []LeadInput{}},
	}
	confirmed := importCost("拍摄", "3000.00", "CNY")
	confirmed.NotDuplicateOf = []string{"cost-1"}
	changed = append(changed, ImportInput{RecordKind: "cost", Costs: []CostInput{confirmed}})
	for i, input := range changed {
		if other, _ := importFingerprint(input); other == first {
			t.Errorf("change %d kept the fingerprint", i)
		}
	}
}

// What is refused before the database is touched, each naming its field.
func TestAnImportRequestIsCheckedBeforeTheDatabase(t *testing.T) {
	one := []CostInput{importCost("拍摄", "1.00", "CNY")}
	cases := []struct {
		name  string
		in    ImportInput
		key   string
		field string
	}{
		{"unknown kind", ImportInput{RecordKind: "touch", Costs: one}, "", "record_kind"},
		{"no rows", ImportInput{RecordKind: "cost"}, "", "rows"},
		{"too many rows", ImportInput{RecordKind: "cost", Costs: make([]CostInput, MaxImportRows+1)}, "", "rows"},
		{"key over 255 bytes", ImportInput{RecordKind: "cost", Costs: one}, strings.Repeat("k", 256), IdempotencyKeyField},
		// 86 three-byte characters are 258 bytes but 86 runes: bytes decide.
		{"key over 255 bytes in runes under", ImportInput{RecordKind: "cost", Costs: one}, strings.Repeat("键", 86), IdempotencyKeyField},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkImportRequest(tc.in, tc.key)
			fieldErr, ok := errors.AsType[FieldError](err)
			if !ok || fieldErr.Field != tc.field {
				t.Fatalf("err = %v, want a refusal naming %q", err, tc.field)
			}
		})
	}
	if err := checkImportRequest(ImportInput{RecordKind: "cost", Costs: one}, strings.Repeat("k", 255)); err != nil {
		t.Errorf("a 255-byte key was refused: %v", err)
	}
	var store *ROIStore
	if _, err := store.Import(t.Context(), "ws", "u", ImportInput{RecordKind: "cost", Costs: one}, "", false); !errors.Is(err, ErrStorage) {
		t.Errorf("an unwired store answered %v", err)
	}
}

// A field refusal inside a batch says which row.
func TestARowRefusalNamesItsRow(t *testing.T) {
	err := atRow(FieldError{Field: "currency", Reason: "not a supported currency"}, 2)
	fieldErr, ok := errors.AsType[FieldError](err)
	if !ok || fieldErr.Row != 2 || fieldErr.Field != "currency" {
		t.Fatalf("got %#v", err)
	}
	if !errors.Is(atRow(ErrNotFound, 3), ErrNotFound) {
		t.Error("a missing reference stopped being ErrNotFound")
	}
}

// The response is made from the batch alone, so the same batch always
// answers with the same bytes, and nothing about the request that asked (a
// replay flag, a new time) can leak into it.
func TestTheImportResponseIsMadeFromTheBatchAlone(t *testing.T) {
	batch := ImportBatch{
		ImportBatchID: "batch-1", WorkspaceID: "ws", RecordKind: ImportDeals,
		RowCount: 2, WrittenCount: 1, SkippedCount: 1,
		Rows: []ImportRowResult{
			{Row: 1, Outcome: OutcomeWritten, RecordID: "deal-1", DuplicateOf: []string{}},
			{Row: 2, Outcome: OutcomeDuplicate, DuplicateOf: []string{"deal-0"}},
		},
		IdempotencyKey: "k", RecordedBy: "u", CreatedAt: time.Date(2026, 9, 25, 1, 2, 3, 456000000, time.UTC),
	}
	first, _ := json.Marshal(importResponse(batch))
	second, _ := json.Marshal(importResponse(batch))
	if string(first) != string(second) {
		t.Fatalf("one batch, two answers:\n%s\n%s", first, second)
	}
	for _, want := range []string{`"import_batch_id":"batch-1"`, `"dry_run":false`, `"written_count":1`,
		`"duplicate_of":["deal-0"]`, `"created_at":"2026-09-25T01:02:03.456Z"`} {
		if !strings.Contains(string(first), want) {
			t.Errorf("response lacks %s: %s", want, first)
		}
	}
	if strings.Contains(string(first), "duplicate_of_rows") {
		t.Errorf("a real import's response carries the dry-run-only field: %s", first)
	}
}
