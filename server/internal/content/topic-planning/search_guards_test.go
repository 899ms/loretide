package topicplanning

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// specs/036 contract §10: text guards over this package's search_*.go
// sources (T012; FR-001, FR-005, FR-081, FR-101, FR-106; SC-013). Written
// the way marketing_node_guards_test.go is: a property of the code, true on
// every run, not a behaviour a test happened to exercise.

type searchSource struct {
	name string
	text string
	file *ast.File
}

func searchSources(t *testing.T) []searchSource {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(current), "search_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	sources := []searchSource{}
	fset := token.NewFileSet()
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		file, parseErr := parser.ParseFile(fset, path, body, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		sources = append(sources, searchSource{name: filepath.Base(path), text: string(body), file: file})
	}
	if len(sources) == 0 {
		t.Fatal("no search_*.go source files found; the guards below would check nothing")
	}
	return sources
}

// Contract §3: each set is exactly this big. Adding a value is a contract
// change - and for intent and origin a migration - not an edit here.
func TestSearchControlledSetsHaveExactlyTheirSize(t *testing.T) {
	for name, got := range map[string]int{
		"SearchIntents":  len(SearchIntents),
		"ThemeOrigins":   len(ThemeOrigins),
		"DataOrigins":    len(DataOrigins),
		"UnknownReasons": len(UnknownReasons),
	} {
		want := map[string]int{"SearchIntents": 6, "ThemeOrigins": 3, "DataOrigins": 1, "UnknownReasons": 1}[name]
		if got != want {
			t.Errorf("%s has %d values, want exactly %d", name, got, want)
		}
	}
	if SearchIntents[len(SearchIntents)-1] != IntentUnclassified {
		t.Errorf("intent has no unclassified: a person must never have to force a choice (Q7)")
	}
}

// FR-106: the theme table is insert-only, and something does insert into it.
func TestSearchThemeRevisionsHaveNoUpdateOrDeletePath(t *testing.T) {
	inserted := false
	for _, source := range searchSources(t) {
		flat := whitespace.ReplaceAllString(strings.ToUpper(source.text), " ")
		for _, forbidden := range []string{
			"UPDATE CONTENT_SEARCH_", "DELETE FROM CONTENT_SEARCH_", "DO UPDATE", "TRUNCATE",
		} {
			if strings.Contains(flat, forbidden) {
				t.Errorf("%s contains forbidden path %q", source.name, forbidden)
			}
		}
		if strings.Contains(flat, "INSERT INTO CONTENT_SEARCH_THEME_REVISION") {
			inserted = true
		}
	}
	if !inserted {
		t.Fatal("no search file inserts a theme revision; the guard above would pass on nothing")
	}
}

// FR-001 / FR-005 / FR-101 / SC-013: no outbound request, no executor, and
// none of the content modules topic-planning must not reach directly.
func TestSearchCodeImportsNoNetworkExecutorOrForeignModule(t *testing.T) {
	forbidden := []string{
		"net/http", "net/rpc", "net/smtp",
		"/content/agent-workflow", "/content/agent-gateway",
		"/content/work-editor", "/content/feedback-learning", "/content/source-inbox",
		"anthropic", "openai",
	}
	for _, source := range searchSources(t) {
		for _, spec := range source.file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			for _, bad := range forbidden {
				if strings.Contains(strings.ToLower(path), bad) {
					t.Errorf("%s imports %s", source.name, path)
				}
			}
		}
	}
}

// rankingPromises are FR-081's words: no answer the product gives may
// promise or predict a ranking.
var rankingPromises = []string{"保证排名", "上首页", "提升排名", "排名第一", "guarantee", "boost ranking", "top rank"}

// FR-081: no Go string literal in the search sources promises a ranking.
// The one negative rule id is let through.
func TestSearchStringsPromiseNoRanking(t *testing.T) {
	for _, source := range searchSources(t) {
		ast.Inspect(source.file, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || value == "optimization.no_ranking_promise" {
				return true
			}
			lower := strings.ToLower(value)
			for _, word := range rankingPromises {
				if strings.Contains(lower, word) {
					t.Errorf("%s has a string promising a ranking: %q", source.name, value)
				}
			}
			return true
		})
	}
}

// ---------------------------------------------------------------------------
// specs/036 PR 2: suggestions (T041; FR-036, FR-051, FR-101, FR-106; SC-008,
// SC-011, SC-013).

// FR-106: the suggestion tables are insert-only (the UPDATE / DELETE / DO
// UPDATE scan above covers every content_search_ table), and each table a
// path writes is written by an INSERT. Adoption is the only writer of the
// effect table; it lands in PR 3 alongside this guard update.
func TestSearchSuggestionTablesAreWrittenOnlyByInsert(t *testing.T) {
	insertedTables := []string{"CONTENT_SEARCH_SUGGESTION_REVISION", "CONTENT_SEARCH_SUGGESTION_DECISION", "CONTENT_SEARCH_SUGGESTION_EFFECT"}
	flat := ""
	for _, source := range searchSources(t) {
		flat += whitespace.ReplaceAllString(strings.ToUpper(source.text), " ") + "\n"
	}
	for _, table := range insertedTables {
		if !strings.Contains(flat, "INSERT INTO "+table) {
			t.Errorf("no search file inserts into %s; the insert-only guard would pass on nothing", table)
		}
	}
	if !strings.Contains(flat, "FROM CONTENT_SEARCH_SUGGESTION_EFFECT") {
		t.Error("no search file reads content_search_suggestion_effect; the derived state would ignore effects")
	}
}

// packageFuncs is every function and method declared in this package's
// non-test sources, by name.
func packageFuncs(t *testing.T) map[string][]*ast.FuncDecl {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(current), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string][]*ast.FuncDecl{}
	fset := token.NewFileSet()
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				funcs[fn.Name.Name] = append(funcs[fn.Name.Name], fn)
			}
		}
	}
	return funcs
}

// FR-051 / SC-008 / contract §10: the abandon path - its own body and every
// function of this package it can reach, by name - calls no method named
// Apply (the SearchWorks write PR 3 adds) and runs no SQL that writes
// anything but the decision. Scanned by name rather than by type so that a
// write reached through a type assertion or a helper is still found.
func TestAbandonPathWritesNothingButTheDecision(t *testing.T) {
	funcs := packageFuncs(t)
	const root = "abandonSearchSuggestion"
	if len(funcs[root]) == 0 {
		t.Fatalf("no %s in the package; the guard would check nothing", root)
	}
	insert := regexp.MustCompile(`(?i)\bINSERT\s+INTO\s+(\w+)`)
	visited := map[string]bool{root: true}
	queue := []string{root}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, fn := range funcs[name] {
			ast.Inspect(fn, func(node ast.Node) bool {
				switch node := node.(type) {
				case *ast.CallExpr:
					called := ""
					switch fun := node.Fun.(type) {
					case *ast.Ident:
						called = fun.Name
					case *ast.SelectorExpr:
						called = fun.Sel.Name
					}
					if called == "Apply" {
						t.Errorf("%s (reached from %s) calls Apply", name, root)
					}
					if len(funcs[called]) > 0 && !visited[called] {
						visited[called] = true
						queue = append(queue, called)
					}
				case *ast.BasicLit:
					if node.Kind != token.STRING {
						return true
					}
					value, err := strconv.Unquote(node.Value)
					if err != nil {
						return true
					}
					for _, found := range insert.FindAllStringSubmatch(value, -1) {
						if !strings.EqualFold(found[1], "content_search_suggestion_decision") {
							t.Errorf("%s (reached from %s) inserts into %s", name, root, found[1])
						}
					}
					upper := strings.ToUpper(value)
					if strings.Contains(upper, "UPDATE ") && !strings.Contains(upper, "FOR UPDATE") || strings.Contains(upper, "DELETE FROM") {
						t.Errorf("%s (reached from %s) runs %q", name, root, value)
					}
				}
				return true
			})
		}
	}
	if !visited["begin"] || !visited["audit"] {
		t.Fatalf("the abandon path does not reach begin and audit (reached %v); the walk is broken", visited)
	}
}

// FR-101: no file of this package - tests included - imports work-editor or
// feedback-learning. What topic-planning knows of a document comes through
// SearchWorks.
func TestTopicPlanningImportsNoWorkEditorAnywhere(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(current), "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range matches {
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			if strings.Contains(imported, "/content/work-editor") || strings.Contains(imported, "/content/feedback-learning") {
				t.Errorf("%s imports %s", filepath.Base(path), imported)
			}
		}
	}
	if len(matches) < 10 {
		t.Fatalf("found %d Go files; the scan checked too little", len(matches))
	}
}

// jsonNames is every JSON member name a type can carry, embedded structs and
// nested types included.
func jsonNames(typ reflect.Type, seen map[reflect.Type]bool) []string {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct || seen[typ] {
		return nil
	}
	seen[typ] = true
	names := []string{}
	for i := range typ.NumField() {
		field := typ.Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			names = append(names, name)
		}
		names = append(names, strings.ToLower(field.Name))
		names = append(names, jsonNames(field.Type, seen)...)
	}
	return names
}

// FR-036 / SC-011: no suggestion, comparison or theme type - request or
// response - has a field for a score, a rank, a keyword density or count, an
// SEO grade or a recommendation.
func TestSearchTypesCarryNoScoreRankOrKeywordCount(t *testing.T) {
	forbidden := []string{"score", "rank", "density", "keyword_count", "keywordcount", "seo", "recommend", "best", "current_rank"}
	for _, value := range []any{
		SuggestionView{}, SuggestionComparison{}, SuggestionRequest{}, SuggestionRevisionRequest{},
		DecisionRequest{}, suggestionCreateWire{}, suggestionRevisionWire{}, decisionWire{},
		SearchThemeView{}, themeCreateWire{}, themeRevisionWire{},
	} {
		typ := reflect.TypeOf(value)
		names := jsonNames(typ, map[reflect.Type]bool{})
		if len(names) < 3 {
			t.Fatalf("%s: read %d names; the scan is broken", typ.Name(), len(names))
		}
		for _, name := range names {
			for _, word := range forbidden {
				if strings.Contains(strings.ToLower(name), word) {
					t.Errorf("%s has a field %q", typ.Name(), name)
				}
			}
		}
	}
}
