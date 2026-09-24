package feedbacklearning

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// What the ROI records must not be able to do (specs/034 T016: SC-012,
// SC-013, FR-073). Like guards_test.go, these scan SOURCE: "no row was ever
// rewritten" is trivially true of an empty table, "no code here can rewrite
// one" is the claim.

var roiTables = []string{
	"content_roi_cost_revision",
	"content_roi_lead_revision",
	"content_roi_touch_revision",
	"content_roi_deal_revision",
	"content_roi_adjustment_revision",
}

// roiSourceFiles returns the non-test roi_*.go files by name and content.
func roiSourceFiles(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(moduleDir(t), "roi_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[filepath.Base(path)] = string(body)
	}
	for _, required := range []string{"roi_contract.go", "roi_money.go", "roi_dedupe.go", "roi_records.go"} {
		if _, ok := files[required]; !ok {
			t.Fatalf("%s is missing; every guard below would pass vacuously", required)
		}
	}
	return files
}

// FR-013 / FR-073: append-only. No UPDATE and no DELETE against any of the
// five tables anywhere in the module - the workspace delete chain lives in
// pkg/db/queries, not here - AND an INSERT into each, so a module that lost
// its writes cannot pass.
func TestROITablesHaveAnInsertAndNoUpdateOrDelete(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	for _, table := range roiTables {
		name := strings.ToUpper(table)
		for _, forbidden := range []string{"UPDATE " + name, "DELETE FROM " + name, "TRUNCATE " + name} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("append-only table has a %q path", forbidden)
			}
		}
		if !strings.Contains(upper, "INSERT INTO "+name) {
			t.Errorf("no INSERT INTO %s; this guard would pass vacuously", table)
		}
	}
}

// FR-004 / SC-012: money never touches a float. Comments are stripped so
// this sentence does not trip it.
func TestROIMoneyCodeHasNoFloat(t *testing.T) {
	for name, body := range roiSourceFiles(t) {
		code := stripGoComments(body)
		for _, forbidden := range []string{"float32", "float64", "ParseFloat", "math.Round", "big.Float", "NewFloat"} {
			if strings.Contains(code, forbidden) {
				t.Errorf("%s contains %q; amounts are integers and rationals only", name, forbidden)
			}
		}
	}
}

// FR-021 / FR-060: nothing reaches outside the process, and nothing calls a
// model. The module-wide guard covers these files too; this one names them
// so the ROI files cannot be excluded from it by a rename.
func TestROIRecordsMakeNoOutboundCall(t *testing.T) {
	for name, body := range roiSourceFiles(t) {
		for _, forbidden := range []string{
			`"net/http"`, "http.Client", "http.NewRequest", "grpc.", "openai", "anthropic",
			"Completion", "ChatModel", "executor",
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains %q", name, forbidden)
			}
		}
	}
}

// piiTokens are the kinds of customer field FR-016 forbids: the real name,
// phone, messaging handle, mailbox, identity document or postal location.
var piiTokens = []string{"name", "phone", "mobile", "wechat", "email", "idcard", "id_card", "address"}

func containsPII(identifier string) string {
	lower := strings.ToLower(identifier)
	for _, token := range piiTokens {
		if strings.Contains(lower, token) {
			return token
		}
	}
	return ""
}

// SC-013 / FR-016: no column of the five tables is a customer identity field.
// Found by table name, not migration number, so a renumbering keeps it
// pointed at the right files.
func TestROIMigrationsHaveNoCustomerIdentityColumn(t *testing.T) {
	migrations := filepath.Join(moduleDir(t), "..", "..", "..", "migrations")
	column := regexp.MustCompile(`(?m)^\s+([a-z_]+)\s+(text|bigint|integer|boolean|timestamptz)\b`)
	checked := 0
	for _, table := range roiTables {
		matches, err := filepath.Glob(filepath.Join(migrations, "*_"+table+".up.sql"))
		if err != nil || len(matches) != 1 {
			t.Fatalf("expected one create migration for %s, found %v (%v)", table, matches, err)
		}
		body, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatal(err)
		}
		columns := column.FindAllStringSubmatch(string(body), -1)
		if len(columns) < 5 {
			t.Fatalf("read %d columns out of %s; the check below would pass vacuously", len(columns), matches[0])
		}
		for _, match := range columns {
			checked++
			if token := containsPII(match[1]); token != "" {
				t.Errorf("%s has column %q (%s)", table, match[1], token)
			}
		}
	}
	t.Logf("checked %d columns", checked)
}

// SC-013 / FR-016, the Go half: no struct field in the ROI files, by Go name
// or by JSON name, is a customer identity field.
func TestROIStructsHaveNoCustomerIdentityField(t *testing.T) {
	fset := token.NewFileSet()
	checked := 0
	for name, body := range roiSourceFiles(t) {
		file, err := parser.ParseFile(fset, name, body, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			structType, ok := node.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range structType.Fields.List {
				identifiers := []string{}
				for _, ident := range field.Names {
					identifiers = append(identifiers, ident.Name)
				}
				if field.Tag != nil {
					tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`")).Get("json")
					identifiers = append(identifiers, strings.Split(tag, ",")[0])
				}
				for _, identifier := range identifiers {
					checked++
					if token := containsPII(identifier); token != "" {
						t.Errorf("%s: field %q (%s)", name, identifier, token)
					}
				}
			}
			return true
		})
	}
	if checked < 50 {
		t.Fatalf("checked only %d fields; the guard would pass vacuously", checked)
	}
}

// jsonTags lists a struct value's JSON field names.
func jsonTags(value any) []string {
	var tags []string
	typ := reflect.TypeOf(value)
	for i := range typ.NumField() {
		if tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]; tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}
