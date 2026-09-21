package workeditor

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// What this package must NOT be able to do. These scan the SOURCE, not the
// data: "no work ever had its historical_import flipped" is trivially true of
// an empty database, while "no code here can flip one" is the claim FR-018
// makes.

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

// Guard 1. historical_import is decided when the work is created and never
// afterwards (specs/031 FR-015, FR-018), the same rule source-inbox gives its
// own copy of the flag.
//
// content_work is NOT append-only - RenameWork updates the title - so the
// claim cannot be "no UPDATE exists". It is "no UPDATE names this column in
// its SET list". Only the assignment list is scanned: WHERE names work_id
// legitimately and RETURNING names every column, so reading whole statements
// would report every UPDATE as a violation.
//
// Note the two vacuity assertions at the end. A guard that only says "no
// UPDATE assigns it" is green against a package with no UPDATEs at all, and
// one that never writes the column in the first place; both are ways this
// guard survives a refactor that removed the thing it guards.
func TestNoUpdateAssignsHistoricalImport(t *testing.T) {
	sources := moduleSources(t)
	updates := 0
	for _, statement := range sqlLiterals(t, sources) {
		upper := strings.ToUpper(statement)
		if !strings.Contains(upper, "UPDATE CONTENT_WORK ") && !strings.Contains(upper, "UPDATE CONTENT_WORK\n") {
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
		if regexp.MustCompile(`(^|[\s,(])HISTORICAL_IMPORT\s*=`).MatchString(assignments) {
			t.Errorf("an UPDATE assigns the immutable column historical_import:\n%s", statement)
		}
	}
	if updates == 0 {
		t.Fatal("no UPDATE of content_work at all; this guard would pass vacuously")
	}
	if !strings.Contains(strings.ToUpper(sources), "INSERT INTO CONTENT_WORK") {
		t.Fatal("no INSERT into content_work; this guard would pass vacuously")
	}
	if !regexp.MustCompile(`(?i)historical_import`).MatchString(sources) {
		t.Fatal("historical_import is not written anywhere; this guard would pass vacuously")
	}
}

// Guard 2. The version table is append-only. SOP 7.1 requires that approved,
// handed over and published versions are always kept, and a history that can
// be rewritten promises nothing of the sort.
//
// Adding the imported action (specs/031 FR-014) is a new way to APPEND a
// version. It must not have introduced a way to change one.
func TestTheVersionTableHasNoUpdateOrDeletePath(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	for _, forbidden := range []string{"UPDATE CONTENT_ARTIFACT_VERSION", "DELETE FROM CONTENT_ARTIFACT_VERSION"} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("append-only table has a %q path", forbidden)
		}
	}
	if !strings.Contains(upper, "INSERT INTO CONTENT_ARTIFACT_VERSION") {
		t.Fatal("no INSERT into content_artifact_version; this guard would pass vacuously")
	}
}

// Guard 3. A caller cannot choose a version's source or action.
//
// version.go's own comment says they "are chosen by the code above, not by a
// caller". Two things make that true and this guard checks both, because
// either one alone can be removed without the other noticing:
//
//   - versionIntent's fields are unexported, so no package outside this one
//     can build an intent at all. That is what stops a handler from decoding
//     an action off a request body and passing it through.
//   - each entry point hard-codes its own pair. The risk the import path adds
//     is the obvious shortcut - one ImportVersion(source, action) taking them
//     as parameters so the adapter can say what it did, which would let
//     anything claim any version was 'adopted'.
//
// ArtifactVersion's json:"source" / json:"action" tags are deliberately NOT
// forbidden: they serialise the stored row on the way OUT, which is how a
// reader learns what happened. Only the way in is closed.
func TestSourceAndActionAreNeverReadFromInput(t *testing.T) {
	sources := moduleSources(t)
	for _, unexported := range []string{"\tsource Source", "\taction Action"} {
		if !strings.Contains(sources, unexported) {
			t.Fatalf("versionIntent no longer has an unexported %q; another package could now choose a version's provenance", strings.TrimSpace(unexported))
		}
	}
	if regexp.MustCompile(`func \(s \*Store\) \w*Version\(ctx context\.Context,[^)]*\b(source|action) `).MatchString(sources) {
		t.Error("a Store version method takes source or action as a parameter; they are chosen by this package, never supplied")
	}
	// The four entry points, each naming its own pair literally. If one of
	// these disappears the guard above is guarding a path that no longer
	// exists.
	for _, required := range []string{
		"source: SourceEdited, action: ActionSaved",
		"source: SourceEdited, action: ActionRestored",
		"source: SourceAdopted, action: ActionAdopted",
		"source: SourceEdited, action: ActionImported",
	} {
		if !strings.Contains(sources, required) {
			t.Fatalf("no entry point hard-codes %q; this guard would pass vacuously", required)
		}
	}
}

// Guard 4. Nothing here generates content. Constitution IX: real executors
// stay disabled, and 'generated' exists in the source set only so that EP-08
// does not have to widen it later.
//
// The import path is the one that would be tempting to "help": summarising a
// pasted body into a title is a model call, and FR-016 says the title is a
// readable prefix of the body instead.
func TestNoPathWritesTheGeneratedSource(t *testing.T) {
	sources := moduleSources(t)
	if strings.Contains(sources, "action: ActionSaved, source: SourceGenerated") ||
		strings.Contains(sources, "source: SourceGenerated") {
		t.Error("a version path writes source=generated; nothing in this phase may (constitution IX)")
	}
	for _, forbidden := range []string{"openai", "anthropic", "CompletionRequest", `"net/http"`} {
		if strings.Contains(sources, forbidden) {
			t.Errorf("module source contains %q; this package calls no model and makes no request", forbidden)
		}
	}
	// SourceGenerated must still EXIST - the guard above is about writing it,
	// not about deleting it from the set.
	if !strings.Contains(sources, "SourceGenerated Source =") {
		t.Fatal("SourceGenerated is gone from the controlled set; EP-08 would have to widen it again")
	}
}
