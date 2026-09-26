package feedbacklearning

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// What the search half of this module must not be able to do (specs/036
// T087; FR-005, FR-075, FR-077, FR-078, FR-081, FR-106; contract §10). Like
// guards_test.go these scan SOURCE: "no observation was ever rewritten" is
// trivially true of an empty table, "no code here can rewrite one" is the
// claim.

var searchTables = []string{"content_search_metric", "content_search_rank_observation_revision"}

// searchSourceFiles returns the non-test search_*.go files by name.
func searchSourceFiles(t *testing.T) map[string]string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(moduleDir(t), "search_*.go"))
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
	for _, required := range []string{"search_contract.go", "search_ports.go", "search_metric.go", "search_observation.go"} {
		if _, ok := files[required]; !ok {
			t.Fatalf("%s is missing; every guard below would pass vacuously", required)
		}
	}
	return files
}

// FR-106: both tables are append-only. No UPDATE, DELETE or TRUNCATE against
// either anywhere in the module - the workspace delete chain lives in
// pkg/db/queries - no upsert, AND an INSERT into each, so a module that lost
// its writes cannot pass.
func TestSearchTablesHaveAnInsertAndNoUpdateOrDelete(t *testing.T) {
	upper := strings.ToUpper(strings.ReplaceAll(moduleSources(t), "\r\n", "\n"))
	for _, table := range searchTables {
		name := strings.ToUpper(table)
		for _, forbidden := range []string{"UPDATE " + name, "DELETE FROM " + name, "TRUNCATE " + name} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("append-only table has a %q path", forbidden)
			}
		}
		if !strings.Contains(upper, "INSERT INTO "+name+"\n") && !strings.Contains(upper, "INSERT INTO "+name+" ") {
			t.Fatalf("no INSERT into %s; this guard would pass vacuously", table)
		}
	}
	for name, body := range searchSourceFiles(t) {
		if strings.Contains(strings.ToUpper(body), "ON CONFLICT") {
			t.Errorf("%s has an ON CONFLICT clause", name)
		}
	}
}

// FR-077 / 027's package guard, restated for the search files: their SQL
// sums, averages, groups and counts nothing. Ordering is not aggregating.
func TestSearchSQLAggregatesNothing(t *testing.T) {
	statements := 0
	for name, body := range searchSourceFiles(t) {
		for _, statement := range sqlLiterals(t, body) {
			upper := strings.ToUpper(statement)
			if !strings.Contains(upper, "CONTENT_") {
				continue
			}
			statements++
			for _, forbidden := range []string{"SUM(", "AVG(", "MIN(", "MAX(", "COUNT(", "GROUP BY", "RANK(", "PERCENTILE", "MEDIAN"} {
				if strings.Contains(upper, forbidden) {
					t.Errorf("%s aggregates (%s) in:\n%s", name, forbidden, statement)
				}
			}
			// And no statement names another module's table: the references
			// are the adapters' to answer (FR-102).
			for _, word := range strings.FieldsFunc(strings.ToLower(statement), func(r rune) bool {
				return !(r == '_' || r >= 'a' && r <= 'z')
			}) {
				if strings.HasPrefix(word, "content_") && word != searchTables[0] && word != searchTables[1] {
					t.Errorf("%s names table %s in:\n%s", name, word, statement)
				}
			}
		}
	}
	if statements < 5 {
		t.Fatalf("found %d search statements; the guard would pass vacuously", statements)
	}
}

// FR-078 / FR-101: no file of feedback-learning - tests included - imports
// topic-planning. The theme is checked through handler/content_search_
// observations.go.
func TestFeedbackLearningDoesNotImportTopicPlanning(t *testing.T) {
	entries, err := os.ReadDir(moduleDir(t))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(moduleDir(t), entry.Name()), nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		checked++
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			for _, forbidden := range []string{"/content/topic-planning", "/content/ip-profile", "/content/work-editor", "/handler"} {
				if strings.HasSuffix(path, forbidden) || strings.Contains(path, forbidden+"/") {
					t.Errorf("%s imports %s", entry.Name(), path)
				}
			}
		}
	}
	if checked < 10 {
		t.Fatalf("parsed %d files; the guard would pass vacuously", checked)
	}
}

// FR-005 / FR-084: the search files make no outbound request and call no
// model or executor.
func TestSearchFilesMakeNoOutboundCall(t *testing.T) {
	for name, body := range searchSourceFiles(t) {
		for _, forbidden := range []string{
			`"net/http"`, "http.Get", "http.Post", "http.NewRequest", "http.Client",
			"/content/agent-workflow", "/content/agent-gateway", "openai", "anthropic", "CompletionRequest",
		} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s contains %q", name, forbidden)
			}
		}
	}
}

// FR-081 / D4: no Go string literal in the search files promises or predicts
// a ranking. Rule ids are ids; the one negative id the contract allows is
// topic-planning's, not this module's.
func TestSearchLiteralsMakeNoRankingPromise(t *testing.T) {
	promises := []string{"保证排名", "上首页", "提升排名", "排名第一", "guarantee", "boost ranking", "top rank", "rank first", "first page"}
	literals := 0
	fset := token.NewFileSet()
	for name, body := range searchSourceFiles(t) {
		file, err := parser.ParseFile(fset, name, body, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			literals++
			value := strings.ToLower(literal.Value)
			for _, promise := range promises {
				if strings.Contains(value, promise) {
					t.Errorf("%s: a string literal promises a ranking: %s", name, literal.Value)
				}
			}
			return true
		})
	}
	if literals < 20 {
		t.Fatalf("found %d literals; the guard would pass vacuously", literals)
	}
}
