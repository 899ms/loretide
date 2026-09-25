package feedbacklearning

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// What the operating diagnosis must not be able to do (specs/035 T015:
// FR-010, FR-068, FR-070, FR-082 to FR-086, FR-090; SC-011 to SC-013;
// contract §9). Like guards_test.go these scan SOURCE: "no version was ever
// rewritten" is trivially true of an empty table, "no code here can rewrite
// one" is the claim.

var opdiagTables = []string{
	"content_opdiag_report_version",
	"content_opdiag_work_mark",
}

// opdiagSourceFiles returns the non-test opdiag_*.go files by name.
func opdiagSourceFiles(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(moduleDir(t), "opdiag_*.go"))
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
	for _, required := range []string{"opdiag_contract.go", "opdiag_inputs.go", "opdiag_calc.go",
		"opdiag_report.go", "opdiag_marks.go"} {
		if _, ok := files[required]; !ok {
			t.Fatalf("%s is missing; every guard below would pass vacuously", required)
		}
	}
	return files
}

// handlerOpdiagFiles returns server/internal/handler/content_opdiag_*.go,
// tests excluded.
func handlerOpdiagFiles(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(moduleDir(t), "..", "..", "handler", "content_opdiag_*.go"))
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
	if _, ok := files["content_opdiag_reports.go"]; !ok {
		t.Fatal("handler/content_opdiag_reports.go is missing; the guard would pass vacuously")
	}
	return files
}

// FR-040 / FR-086: both tables are append-only. No UPDATE, DELETE or
// TRUNCATE against either anywhere in the module - the workspace delete
// chain lives in pkg/db/queries - no upsert, AND an INSERT into each, so a
// module that lost its writes cannot pass.
func TestOpDiagTablesHaveAnInsertAndNoUpdateOrDelete(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	for _, table := range opdiagTables {
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
	// A new version "regenerated" onto an old key would be an upsert.
	if strings.Contains(upper, "DO UPDATE") {
		t.Error("an INSERT ... ON CONFLICT DO UPDATE rewrites an append-only row")
	}
}

// FR-010: the diagnosis never touches a float.
func TestOpDiagCodeHasNoFloat(t *testing.T) {
	for name, body := range opdiagSourceFiles(t) {
		code := stripGoComments(body)
		for _, forbidden := range []string{"float32", "float64", "ParseFloat", "math.Round", "big.Float", "NewFloat"} {
			if strings.Contains(code, forbidden) {
				t.Errorf("%s contains %q; counts are integers and means are rationals", name, forbidden)
			}
		}
	}
}

// FR-083 / contract §9: the only tables the diagnosis's SQL names are its
// own and this module's metrics and excerpts. Everything else is read
// through an adapter.
func TestOpDiagSQLReadsOnlyItsOwnTables(t *testing.T) {
	table := regexp.MustCompile(`\bcontent_[a-z_]+`)
	seen := 0
	for name, body := range opdiagSourceFiles(t) {
		for _, statement := range sqlLiterals(t, body) {
			for _, match := range table.FindAllString(statement, -1) {
				seen++
				if !strings.HasPrefix(match, "content_opdiag_") && match != "content_manual_metric" &&
					match != "content_feedback_excerpt" {
					t.Errorf("%s names table %s in:\n%s", name, match, statement)
				}
			}
		}
	}
	if seen == 0 {
		t.Fatal("no table named in any opdiag statement; the guard would pass vacuously")
	}
	// The metric and excerpt reads reuse the module's own selects.
	sources := moduleSources(t)
	for _, required := range []string{"FROM content_manual_metric", "FROM content_feedback_excerpt"} {
		if !strings.Contains(sources, required) {
			t.Fatalf("%q is gone from the module", required)
		}
	}
}

// FR-082 / SC-009 / US7 scenario 4: no diagnosis path reads sources or
// knowledge - neither the module files nor the handler adapter.
func TestOpDiagReadsNoSourceOrKnowledge(t *testing.T) {
	files := opdiagSourceFiles(t)
	for name, body := range handlerOpdiagFiles(t) {
		files["handler/"+name] = body
	}
	for name, body := range files {
		lower := strings.ToLower(body)
		for _, forbidden := range []string{"source-inbox", "sourceinbox", "knowledge-base", "knowledgebase"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s refers to %q", name, forbidden)
			}
		}
	}
}

// FR-070 / FR-071: no model, no runner, nothing out of the process.
func TestOpDiagMakesNoOutboundCallAndCallsNoModel(t *testing.T) {
	for name, body := range opdiagSourceFiles(t) {
		for _, forbidden := range []string{
			`"net/http"`, "http.Client", "http.NewRequest", "grpc.", "openai", "anthropic",
			"Completion", "ChatModel", "executor",
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains %q", name, forbidden)
			}
		}
		if strings.Contains(body, "author_kind") || strings.Contains(strings.ToLower(body), "ai_judgement_revision") {
			t.Errorf("%s has an AI judgement path; PR 1 keeps only the hook key", name)
		}
	}
}

// FR-090 / SC-013: the development diagnostics module never mentions the
// operating diagnosis tables, and this module writes none of its tables.
func TestOpDiagIsNotTheDevelopmentDiagnostics(t *testing.T) {
	dir := filepath.Join(moduleDir(t), "..", "diagnostics")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		read++
		if strings.Contains(string(body), "content_opdiag_") {
			t.Errorf("diagnostics/%s names an operating diagnosis table", entry.Name())
		}
	}
	if read == 0 {
		t.Fatal("read no diagnostics sources; the guard would pass vacuously")
	}
	for name, body := range opdiagSourceFiles(t) {
		upper := strings.ToUpper(body)
		for _, table := range []string{"CONTENT_TECHNICAL_LOG", "CONTENT_OPERATION_AUDIT", "CONTENT_DIAGNOSTIC"} {
			if strings.Contains(upper, "INSERT INTO "+table) {
				t.Errorf("%s writes %s directly; the audit goes through AuditTx", name, table)
			}
		}
	}
}

// FR-068: no business memory table and no path that writes one.
func TestOpDiagHasNoBusinessMemory(t *testing.T) {
	for name, body := range opdiagSourceFiles(t) {
		lower := strings.ToLower(body)
		for _, forbidden := range []string{"business_memory", "operating_memory", "memory_revision"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s refers to %q", name, forbidden)
			}
		}
	}
	matches, err := filepath.Glob(filepath.Join(moduleDir(t), "..", "..", "..", "migrations", "*_content_opdiag_*.up.sql"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no opdiag migrations found (%v)", err)
	}
	for _, path := range matches {
		if base := filepath.Base(path); strings.Contains(base, "memory") || strings.Contains(base, "ai_") {
			t.Errorf("migration %s", base)
		}
	}
}
