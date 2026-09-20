package reviewdelivery

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// What this package must NOT be able to do. Every one of these scans the
// SOURCE, not the data: "no publication was ever pushed to a platform" is
// trivially true of an empty database, while "no code here can push one" is
// the claim SOP 9.2 actually makes.

func moduleDir(t *testing.T) string {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	return filepath.Dir(current)
}

// moduleSources returns the package's non-test Go files, joined.
func moduleSources(t *testing.T) string {
	t.Helper()
	dir := moduleDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var combined strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(dir, name))
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

// The two append-only tables. Note the INSERT assertions: a guard that only
// says "no UPDATE, no DELETE" is green against a module that writes no SQL at
// all, which is how such a guard survives a refactor that deleted the writes.
func TestTheAppendOnlyTablesHaveNoUpdateOrDeletePath(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	for _, table := range []string{"CONTENT_PUBLICATION_RECORD", "CONTENT_REVIEW_TRANSITION"} {
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

// The frozen snapshot. It lives on a row whose status column IS updated, so
// "the table is append-only" cannot be the claim - "no UPDATE names this
// column" is.
//
// The scan is over whole SQL literals rather than single lines. A first version
// of this test looked at one line at a time and missed
// `snapshot = snapshot || jsonb_build_object(...)` sitting two lines under its
// SET, which is exactly the shape a well-meaning change would take.
func TestNoUpdateStatementNamesTheFrozenSnapshotColumn(t *testing.T) {
	statements := sqlLiterals(t, moduleSources(t))
	if len(statements) == 0 {
		t.Fatal("no SQL literals found; this guard would pass vacuously")
	}
	updates := 0
	for _, statement := range statements {
		upper := strings.ToUpper(statement)
		if !strings.Contains(upper, "UPDATE CONTENT_REVIEW_REQUEST") {
			continue
		}
		updates++
		// Only the assignment list. The RETURNING clause names the column
		// legitimately - the caller gets the row back, snapshot included.
		assignments := upper
		if start := strings.Index(assignments, "SET"); start >= 0 {
			assignments = assignments[start:]
		}
		if end := strings.Index(assignments, "WHERE"); end >= 0 {
			assignments = assignments[:end]
		}
		if strings.Contains(assignments, "SNAPSHOT") {
			t.Errorf("an UPDATE assigns the frozen snapshot column:\n%s", statement)
		}
	}
	if updates == 0 {
		t.Fatal("no UPDATE of content_review_request at all; this guard would pass vacuously")
	}
}

// sqlLiterals returns the contents of every backtick-quoted string in the
// source, which is how every statement in this package is written.
func sqlLiterals(t *testing.T, sources string) []string {
	t.Helper()
	var literals []string
	parts := strings.Split(sources, "`")
	for i := 1; i < len(parts); i += 2 {
		literals = append(literals, parts[i])
	}
	return literals
}

// SOP 9.2: "系统不保存平台发布密钥，也不提供发布执行接口", plus this card's own
// stronger rule that no outbound request of any kind is made.
func TestNothingInThisModuleReachesOutsideTheProcess(t *testing.T) {
	sources := moduleSources(t)
	for _, forbidden := range []string{
		// Outbound HTTP of any shape.
		`"net/http"`, "http.Get", "http.Post", "http.NewRequest", "http.Client",
		// A platform SDK or credential would arrive as one of these.
		"oauth", "AccessToken", "access_token", "api_secret", "APISecret",
		// A model call.
		"openai", "anthropic", "llm.", "CompletionRequest",
	} {
		if strings.Contains(sources, forbidden) {
			t.Errorf("module source contains %q; this card makes no outbound call, holds no platform credential and calls no model", forbidden)
		}
	}
}

// SOP 9.2 again, from the other side: there is no method here whose name says
// it publishes. Record() writes down what a person says happened; it does not
// make it happen.
func TestThereIsNoPublishExecutionInterface(t *testing.T) {
	sources := moduleSources(t)
	forbidden := regexp.MustCompile(`func \([^)]*\) (Publish|PostTo|SendTo|Upload)[A-Za-z]*\(`)
	if match := forbidden.FindString(sources); match != "" {
		t.Errorf("module exposes a publish execution interface: %s", match)
	}
}

// Ruling Q5: SOP 9.2 gives no enumeration for who declared a publication or for
// how it was verified, so this card defines none.
//
// This is the guard for the day someone reads two free text fields and decides
// that "completing" them is helpful. The six sets below are the whole list.
func TestThereIsNoSeventhControlledSet(t *testing.T) {
	sources := moduleSources(t)
	declaration := regexp.MustCompile(`var ([A-Z][A-Za-z]*) = \[\]([A-Z][A-Za-z]*)\{`)
	found := map[string]bool{}
	for _, match := range declaration.FindAllStringSubmatch(sources, -1) {
		found[match[1]] = true
	}
	want := []string{
		"Channels", "ReviewStatuses", "DeliveryStatuses", "PublicationStatuses",
		"HandoffMethods", "VersionMatches", "SubjectKinds",
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("controlled set %s is gone", name)
		}
		delete(found, name)
	}
	for extra := range found {
		t.Errorf("a controlled set %q was added; SOP 9.2 enumerates neither the declarer nor the verification method, and inventing one puts words in the SOP's mouth that every stored row then has to be valid against", extra)
	}
	// And the two fields themselves are still plain strings.
	for _, field := range []string{"DeclaredBy", "VerificationNote"} {
		plain := regexp.MustCompile(field + ` +string`)
		if !plain.MatchString(sources) {
			t.Errorf("%s is no longer free text", field)
		}
	}
}

// Ruling Q4: the channel set is defined here because review-delivery's declared
// dependencies do not include ip-profile. That makes drift possible, so the
// test reads ip-profile's source instead of importing it.
func TestTheChannelSetAgreesWithIPProfilesPlatforms(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(moduleDir(t), "..", "ip-profile", "account.go"))
	if err != nil {
		t.Fatal(err)
	}
	platforms := map[string]bool{}
	declaration := regexp.MustCompile(`Platform[A-Za-z]* +Platform += +"([a-z_]+)"`)
	for _, match := range declaration.FindAllStringSubmatch(string(source), -1) {
		platforms[match[1]] = true
	}
	if len(platforms) == 0 {
		t.Fatal("read no platforms out of ip-profile; the comparison below would pass vacuously")
	}
	for _, channel := range Channels {
		if !platforms[string(channel)] {
			t.Errorf("channel %q is not one of ip-profile's platforms %v", channel, platforms)
		}
	}
}

// The document kind is compared as a string for the same reason: work-editor is
// a storage-level dependency here, and the adapter reads the column. The
// constant still has to be work-editor's actual value.
func TestTheChannelDraftKindMatchesWorkEditors(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(moduleDir(t), "..", "work-editor", "contract.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `KindChannelDraft Kind = "`+channelDraftKind+`"`) {
		t.Errorf("work-editor's channel draft kind is no longer %q", channelDraftKind)
	}
}

// SC-010. The planned time is stored and compared; nothing acts on it.
//
// The check is on READERS, not on the column's existence: a scheduler would
// appear as a third place that reads scheduled_at, and this names the two that
// are allowed.
func TestNothingReadsTheScheduledTimeInOrderToAct(t *testing.T) {
	dir := moduleDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		// Stores it, reads it back, and hands it to IsDue.
		"delivery.go": true,
		// Declares the field and holds IsDue itself.
		"states.go": true, "contract.go": true,
		// The SELECT list.
		"store.go": true,
	}
	seen := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(source), "scheduled_at") && !strings.Contains(string(source), "ScheduledAt") {
			continue
		}
		seen = true
		if !allowed[name] {
			t.Errorf("%s reads the planned time; the only uses are storing it and comparing it on read", name)
		}
	}
	if !seen {
		t.Fatal("nothing mentions the planned time at all; this guard would pass vacuously")
	}
}
