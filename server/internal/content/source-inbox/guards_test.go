package sourceinbox

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// What this package must NOT be able to do. Every one of these scans the
// SOURCE, not the data: "no URL was ever fetched" is trivially true of an empty
// database, while "no code here can fetch one" is the claim FR-002 makes.

func moduleDir(t *testing.T) string {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	return filepath.Dir(current)
}

func moduleSources(t *testing.T) string {
	t.Helper()
	entries, err := os.ReadDir(moduleDir(t))
	if err != nil {
		t.Fatal(err)
	}
	var combined strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(moduleDir(t), name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		combined.Write(source)
		combined.WriteString("\n")
	}
	if combined.Len() == 0 {
		t.Fatal("no module sources found; every guard below would pass vacuously")
	}
	return combined.String()
}

// sqlLiterals returns the contents of every backtick-quoted string, which is
// how every statement in this package is written.
func sqlLiterals(t *testing.T, sources string) []string {
	t.Helper()
	var literals []string
	parts := strings.Split(sources, "`")
	for i := 1; i < len(parts); i += 2 {
		literals = append(literals, parts[i])
	}
	return literals
}

// Guard 1. The two append-only tables.
//
// Note the INSERT assertions: a guard that only says "no UPDATE, no DELETE" is
// green against a module that writes no SQL at all, which is how such a guard
// survives a refactor that deleted the writes.
func TestTheAppendOnlyTablesHaveNoUpdateOrDeletePath(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	for _, table := range []string{"CONTENT_SOURCE_SNAPSHOT", "CONTENT_SOURCE_REVISION"} {
		for _, forbidden := range []string{"UPDATE " + table, "DELETE FROM " + table} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("append-only table has a %q path", forbidden)
			}
		}
		if !strings.Contains(upper, "INSERT INTO "+table) {
			t.Fatalf("no INSERT into %s; this guard would pass vacuously", table)
		}
	}
}

// Guard 2. The five columns that describe the act of collecting.
//
// The claim cannot be "the table is append-only" - content_source's mutable
// half is updated on every organise. It is "no UPDATE names one of these in
// its SET list".
//
// Only the assignment list is scanned. WHERE names source_id legitimately and
// RETURNING names every column, so reading the whole statement would report
// every UPDATE as a violation.
func TestNoUpdateAssignsAnImmutableColumn(t *testing.T) {
	immutable := []string{"KIND", "URL", "CAPTURED_AT", "RECORDED_BY", "HISTORICAL_IMPORT"}
	updates := 0
	for _, statement := range sqlLiterals(t, moduleSources(t)) {
		upper := strings.ToUpper(statement)
		if !strings.Contains(upper, "UPDATE CONTENT_SOURCE ") && !strings.Contains(upper, "UPDATE CONTENT_SOURCE\n") {
			continue
		}
		updates++
		assignments := upper
		if start := strings.Index(assignments, "SET"); start >= 0 {
			assignments = assignments[start:]
		}
		if end := strings.Index(assignments, "WHERE"); end >= 0 {
			assignments = assignments[:end]
		}
		for _, column := range immutable {
			// Word-ish boundary: "URL" must not match inside another word, and
			// the SET list is written one assignment per line.
			if regexp.MustCompile(`(^|[\s,(])` + column + `\s*=`).MatchString(assignments) {
				t.Errorf("an UPDATE assigns the immutable column %s:\n%s", column, statement)
			}
		}
	}
	if updates == 0 {
		t.Fatal("no UPDATE of content_source at all; this guard would pass vacuously")
	}
}

// Guard 3. FR-002: a url is stored, never visited.
func TestNothingInThisModuleReachesOutsideTheProcess(t *testing.T) {
	sources := moduleSources(t)
	for _, forbidden := range []string{
		// Outbound HTTP of any shape.
		`"net/http"`, "http.Get", "http.Post", "http.NewRequest", "http.Client",
		// A headless browser or scraper would arrive as one of these.
		"chromedp", "colly", "goquery",
		// A model call.
		"openai", "anthropic", "llm.", "CompletionRequest",
	} {
		if strings.Contains(sources, forbidden) {
			t.Errorf("module source contains %q; this card fetches nothing and calls no model", forbidden)
		}
	}
	// net/url IS imported - parsing a link to check it is well formed is not
	// visiting it. Asserting that keeps the list above honest about what it
	// forbids.
	if !strings.Contains(sources, `"net/url"`) {
		t.Fatal("net/url is no longer imported; the URL validation this guard sits next to is gone")
	}
}

// Guard 4. From the other side: no method here is named as if it fetched.
func TestThereIsNoFetchInterface(t *testing.T) {
	forbidden := regexp.MustCompile(`func \([^)]*\) (Fetch|Crawl|Download|Scrape|Visit)[A-Za-z]*\(`)
	if match := forbidden.FindString(moduleSources(t)); match != "" {
		t.Errorf("module exposes a fetch interface: %s", match)
	}
}

// Guard 5. There are exactly two controlled sets.
//
// This is the guard for the day someone reads SOP §7.1, sees pending →
// processing → ready / partial / failed, and adds it. Ruling Q3 = A: that
// column arrives with the parser. A frozen enum is a claim the code cannot
// back up.
func TestThereIsNoThirdControlledSet(t *testing.T) {
	declaration := regexp.MustCompile(`var ([A-Z][A-Za-z]*) = \[\]([A-Z][A-Za-z]*)\{`)
	found := map[string]bool{}
	for _, match := range declaration.FindAllStringSubmatch(moduleSources(t), -1) {
		found[match[1]] = true
	}
	for _, name := range []string{"Kinds", "Statuses"} {
		if !found[name] {
			t.Errorf("controlled set %s is gone", name)
		}
		delete(found, name)
	}
	for extra := range found {
		t.Errorf("a controlled set %q was added; SOP §7.1's parse status belongs to the parser card, and a set nothing advances is a decoration that reads as a claim", extra)
	}
}

// Guard 6. Nothing merges or removes a duplicate.
//
// R-011: "内容相同不删除独立的收藏上下文与批注". The hint is the whole feature;
// acting on it is the thing this must not grow into.
func TestNothingMergesOrDeletesASource(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	if strings.Contains(upper, "DELETE FROM CONTENT_SOURCE") {
		t.Error("something deletes a source; archiving is a status, and R-011 forbids removing a duplicate's own context")
	}
	forbidden := regexp.MustCompile(`func \([^)]*\) (Merge|Dedupe|Deduplicate|Combine)[A-Za-z]*\(`)
	if match := forbidden.FindString(moduleSources(t)); match != "" {
		t.Errorf("module exposes a merge interface: %s", match)
	}
	if !strings.Contains(moduleSources(t), "func (s *Store) DuplicatesByHash") {
		t.Fatal("the duplicate hint is gone; this guard would pass vacuously")
	}
}
