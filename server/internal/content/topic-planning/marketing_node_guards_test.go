package topicplanning

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// Contract §7. Text guards over the marketing node source files, written the
// way 022's TestBriefStoreHasNoUpdateOrDeletePath is: a property of the code,
// true on every run, rather than a behaviour a test happened to exercise.

// marketingNodeSources returns the upper-cased non-test marketing_node*.go
// files of this package, keyed by file name.
func marketingNodeSources(t *testing.T) map[string]string {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(current), "marketing_node*.go"))
	if err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		sources[filepath.Base(path)] = strings.ToUpper(string(body))
	}
	if len(sources) == 0 {
		t.Fatal("no marketing node source files found; the guards below would check nothing")
	}
	return sources
}

var whitespace = regexp.MustCompile(`\s+`)

// FR-005 / FR-041: the revision table is insert-only.
func TestMarketingNodeRevisionsHaveNoUpdateOrDeletePath(t *testing.T) {
	inserted := false
	for name, source := range marketingNodeSources(t) {
		flat := whitespace.ReplaceAllString(source, " ")
		for _, forbidden := range []string{
			"UPDATE CONTENT_MARKETING_NODE_REVISION",
			"DELETE FROM CONTENT_MARKETING_NODE_REVISION",
		} {
			if strings.Contains(flat, forbidden) {
				t.Errorf("%s contains forbidden path %q", name, forbidden)
			}
		}
		if strings.Contains(flat, "INSERT INTO CONTENT_MARKETING_NODE_REVISION") {
			inserted = true
		}
	}
	if !inserted {
		t.Fatal("no marketing node file inserts a revision; the guard above would pass on nothing")
	}
}

// FR-040 / FR-041: node code changes none of the downstream objects. In PR 1
// there is no card-creating path at all, so content_topic_card is covered by
// the same rule; PR 2's adoption goes through the extracted insert function
// and does not spell the table here either.
func TestMarketingNodeCodeDoesNotWriteDownstreamTables(t *testing.T) {
	downstream := []string{
		"CONTENT_TOPIC_CARD", "CONTENT_BRIEF_REVISION", "CONTENT_START_SNAPSHOT",
		"CONTENT_WORK", "CONTENT_ARTIFACT_VERSION", "CONTENT_REVIEW_",
		"CONTENT_DELIVERY_TASK", "CONTENT_PUBLICATION_RECORD",
	}
	for name, source := range marketingNodeSources(t) {
		flat := whitespace.ReplaceAllString(source, " ")
		for _, table := range downstream {
			for _, verb := range []string{"UPDATE " + table, "DELETE FROM " + table} {
				if strings.Contains(flat, verb) {
					t.Errorf("%s writes a downstream table: %q", name, verb)
				}
			}
		}
	}
}

// Constitution IX: candidates and node text come from people, never from an
// executor. check:content-boundaries already restricts what topic-planning
// may import; this states the reason when it fails.
func TestMarketingNodeCodeImportsNoExecutor(t *testing.T) {
	for name, source := range marketingNodeSources(t) {
		for _, forbidden := range []string{"/AGENT", "EXECUTOR", "/DAEMON", "ANTHROPIC", "OPENAI"} {
			for _, line := range strings.Split(source, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, `"`) && strings.Contains(trimmed, forbidden) {
					t.Errorf("%s imports %s: node text must be written by people (FR-029)", name, trimmed)
				}
			}
		}
	}
}
