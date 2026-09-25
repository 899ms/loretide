package topicplanning

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
