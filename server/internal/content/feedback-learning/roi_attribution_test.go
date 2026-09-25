package feedbacklearning

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// specs/034 PR 2, T045 (the pure half) and T046: the judgement's own checks,
// and the guard that nothing but a person's request fills in a work, an
// evidence type or a judgement (FR-021).

func TestAJudgementIsCheckedByName(t *testing.T) {
	cases := []struct {
		name  string
		in    AttributionInput
		field string
	}{
		{"judgement outside the set", AttributionInput{Judgement: "probably"}, "judgement"},
		{"unknown with a touch", AttributionInput{Judgement: "unknown", TouchIDs: []string{"t-1"}}, "touch_ids"},
		{"repeated touch", AttributionInput{Judgement: "multi_touch", TouchIDs: []string{"t-1", "t-1"}}, "touch_ids"},
		{"empty touch id", AttributionInput{Judgement: "confirmed", TouchIDs: []string{" "}}, "touch_ids"},
		{"weights not one per touch", AttributionInput{Judgement: "multi_touch",
			TouchIDs: []string{"t-1", "t-2"}, Weights: []int64{1}}, "weights"},
		{"zero weight", AttributionInput{Judgement: "multi_touch",
			TouchIDs: []string{"t-1", "t-2"}, Weights: []int64{1, 0}}, "weights"},
		{"note too long", AttributionInput{Judgement: "unknown", Note: strings.Repeat("字", MaxNoteRunes+1)}, "note"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateAttribution(tc.in)
			if fieldErr, ok := errors.AsType[FieldError](err); !ok || fieldErr.Field != tc.field {
				t.Fatalf("err = %v, want a refusal naming %s", err, tc.field)
			}
		})
	}
	for _, in := range []AttributionInput{
		{Judgement: "unknown"},
		{Judgement: "confirmed", TouchIDs: []string{"t-1"}},
		{Judgement: "operator_judgement", TouchIDs: []string{"t-1"}},
		{Judgement: "multi_touch", TouchIDs: []string{"t-1", "t-2"}, Weights: []int64{3, 1}},
		// Judged, but nothing accepted: stays unattributed (FR-035).
		{Judgement: "multi_touch"},
	} {
		record, err := ValidateAttribution(in)
		if err != nil {
			t.Fatalf("%+v refused: %v", in, err)
		}
		if record.TouchIDs == nil || record.Weights == nil {
			t.Fatalf("%+v: nil lists would reach the database as NULL", in)
		}
	}
}

func TestTheJudgementSetIsTheContracts(t *testing.T) {
	if got := setValues(Judgements); !slices.Equal(got, []string{"confirmed", "operator_judgement", "multi_touch", "unknown"}) {
		t.Fatalf("judgements = %v", got)
	}
	if got := setValues(AttributionMethods); !slices.Equal(got, []string{"first_touch", "last_touch", "even_split", "judgement_weights"}) {
		t.Fatalf("attribution methods = %v", got)
	}
	if got := setValues(AllocationTargets); !slices.Equal(got, []string{"work", "account", "campaign_label", "period"}) {
		t.Fatalf("allocation targets = %v", got)
	}
	if got := setValues(AllocationMethods); !slices.Equal(got, []string{"weights", "amounts"}) {
		t.Fatalf("allocation methods = %v", got)
	}
	if got := setValues(ReasonCodes); !slices.Equal(got, []string{"no_data", "currency_unconverted",
		"labor_rate_missing", "missing_gross_profit", "refund_without_gross_delta",
		"missing_denominator", "zero_denominator"}) {
		t.Fatalf("reason codes, in priority order = %v", got)
	}
	if len(MetricIDs) != 16 {
		t.Fatalf("%d metric ids, want the contract's 16", len(MetricIDs))
	}
}

func TestAMinorAmountReadsBackFromItsString(t *testing.T) {
	var minor Minor
	if err := json.Unmarshal([]byte(`"-300000"`), &minor); err != nil || minor != -300000 {
		t.Fatalf("read %d, %v", minor, err)
	}
	for _, bad := range []string{`300000`, `"3000.00"`, `""`, `"-"`, `3.5`} {
		if err := json.Unmarshal([]byte(bad), &minor); err == nil {
			t.Errorf("%s was accepted", bad)
		}
	}
}

// filledFields are the fields that say where a deal came from. Only a
// person's request may set them (FR-021).
var filledFields = []string{"WorkID", "EvidenceType", "Judgement"}

// fillers are the functions allowed to set those fields, or to insert the
// rows that hold them. Every other function in the module is barred. The
// Validate* functions copy a request body; the scan* functions read a stored
// row back; writeTouch and RecordAttribution are the two write paths, each
// behind an endpoint a person calls.
//
// specs/035 adds two that set a WorkID of their own types, not a deal's:
// RecordWorkMark copies the work id of a person's mark from the request, and
// gatherDiagnosisInputs copies a publication record's work id, as
// review-delivery answers it, into a diagnosis input. Neither infers one.
var fillers = []string{
	"RecordAttribution", "RecordWorkMark", "ValidateAttribution", "ValidateCost", "ValidateTouch",
	"gatherDiagnosisInputs", "scanAttribution", "scanTouch", "writeTouch",
}

// T046 / FR-021: nothing in this module fills in a work, an evidence type or
// a judgement except the whitelisted functions. Scanned by function name
// over the module's source, so a new "helpful" inference fails here by name.
func TestOnlyARequestFillsInAWorkEvidenceOrJudgement(t *testing.T) {
	entries, err := os.ReadDir(moduleDir(t))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(moduleDir(t), name), nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fills(fn.Body) {
				found[fn.Name.Name] = true
			}
		}
	}
	got := []string{}
	for name := range found {
		got = append(got, name)
	}
	sort.Strings(got)
	if !slices.Equal(got, fillers) {
		t.Fatalf("functions that set %v or insert touches/judgements = %v, want exactly %v", filledFields, got, fillers)
	}
}

// fills reports whether a function body assigns one of filledFields (as
// x.Field = ... or Field: ... in a literal) or inserts a touch or judgement.
func fills(body *ast.BlockStmt) bool {
	hit := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.AssignStmt:
			for _, target := range n.Lhs {
				if selector, ok := target.(*ast.SelectorExpr); ok && slices.Contains(filledFields, selector.Sel.Name) {
					hit = true
				}
			}
		case *ast.KeyValueExpr:
			if key, ok := n.Key.(*ast.Ident); ok && slices.Contains(filledFields, key.Name) {
				hit = true
			}
		case *ast.BasicLit:
			upper := strings.ToUpper(n.Value)
			if strings.Contains(upper, "INSERT INTO CONTENT_ROI_TOUCH_REVISION") ||
				strings.Contains(upper, "INSERT INTO CONTENT_ROI_ATTRIBUTION_REVISION") {
				hit = true
			}
		}
		return true
	})
	return hit
}
