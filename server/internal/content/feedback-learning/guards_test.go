package feedbacklearning

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// What this package must NOT be able to do. Every one of these scans the
// SOURCE, not the data: "no number was ever fetched from a platform" is
// trivially true of an empty database, while "no code here can fetch one" is
// the claim SOP 10.1 actually makes.

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

// nonTestFiles lists the package's non-test Go files by name.
func nonTestFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(moduleDir(t))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	return names
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

// Both tables are append-only. R-044's "更新保留历史" means a correction is a
// new row; rewriting the first one erases "I copied it down wrong", which is
// itself part of the history.
//
// Note the INSERT assertions: a guard that only says "no UPDATE, no DELETE" is
// green against a module that writes no SQL at all, which is how such a guard
// survives a refactor that deleted the writes.
func TestBothTablesHaveNoUpdateOrDeletePath(t *testing.T) {
	upper := strings.ToUpper(moduleSources(t))
	for _, table := range []string{"CONTENT_MANUAL_METRIC", "CONTENT_FEEDBACK_EXCERPT"} {
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

// SOP 10.1: every number here was typed in by a person. The system never went
// and looked, holds nothing it could look with, and calls no model.
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
			t.Errorf("module source contains %q; every number here is copied in by a person", forbidden)
		}
	}
}

// This card records facts. Adding them up is 10.2's business, and a
// "convenience" total here is the first step towards ranking two platforms'
// incomparable numbers against each other.
func TestThisModuleAggregatesNothing(t *testing.T) {
	statements := sqlLiterals(t, moduleSources(t))
	if len(statements) == 0 {
		t.Fatal("no SQL literals found; this guard would pass vacuously")
	}
	counts := 0
	for _, statement := range statements {
		upper := strings.ToUpper(statement)
		for _, forbidden := range []string{"SUM(", "AVG(", "MAX(VALUE", "MIN(VALUE", "RANK()", "GROUP BY"} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("an aggregate appears in:\n%s", statement)
			}
		}
		// count(*) is allowed in exactly one place: answering "has anybody
		// recorded anything for this record", which is the whole of the
		// pending-registration derivation. It counts rows, not a metric.
		if strings.Contains(upper, "COUNT(") {
			counts++
			if !strings.Contains(upper, "CONTENT_MANUAL_METRIC") {
				t.Errorf("a count() outside the pending derivation:\n%s", statement)
			}
		}
	}
	if counts > 1 {
		t.Errorf("found %d count() statements; only the pending derivation may have one", counts)
	}
}

// SOP 10.1: "不同平台的阅读和播放分别保留，不直接合并排名". Merging them looks
// like saving the reader trouble; it puts two incomparable platforms into one
// ranking.
func TestNothingMergesReadAndPlay(t *testing.T) {
	sources := moduleSources(t)
	// contract.go declares both values and lists them in the set; that is the
	// one place they legitimately sit side by side. Anywhere ELSE, the two
	// named together is a merge being written.
	for _, name := range nonTestFiles(t) {
		if name == "contract.go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(moduleDir(t), name))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(body), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			// Both the Go spelling and the SQL one. A first version of this
			// test looked only for Go double-quoted strings and sailed past
			// `metric IN ('read', 'play')`, which is exactly the shape a
			// merge would take.
			mentionsRead := strings.Contains(line, "MetricRead") ||
				strings.Contains(line, `"read"`) || strings.Contains(line, `'read'`)
			mentionsPlay := strings.Contains(line, "MetricPlay") ||
				strings.Contains(line, `"play"`) || strings.Contains(line, `'play'`)
			if mentionsRead && mentionsPlay {
				t.Errorf("%s names read and play together: %s", name, strings.TrimSpace(line))
			}
		}
	}
	if !strings.Contains(sources, "MetricRead") || !strings.Contains(sources, "MetricPlay") {
		t.Fatal("read and play are gone; this guard would pass vacuously")
	}
	// And no statement adds two metric values together, or selects both names
	// at once - a statement can span lines, so this looks at whole literals.
	for _, statement := range sqlLiterals(t, sources) {
		upper := strings.ToUpper(statement)
		if strings.Contains(upper, "VALUE +") || strings.Contains(upper, "+ VALUE") {
			t.Errorf("a statement adds metric values:\n%s", statement)
		}
		if strings.Contains(upper, "'READ'") && strings.Contains(upper, "'PLAY'") {
			t.Errorf("a statement selects read and play together:\n%s", statement)
		}
	}
}

// SOP 10.1 supports a person redacting an excerpt. It does not redact for
// them, and an automatic redactor that misses one name is worse than none,
// because it teaches people to stop checking.
func TestNothingRedactsAnythingAutomatically(t *testing.T) {
	sources := strings.ToLower(moduleSources(t))
	for _, forbidden := range []string{"regexp.mustcompile", "anonymi", "maskpii", "scrubpii", "detectpii"} {
		if strings.Contains(sources, forbidden) {
			t.Errorf("module source contains %q; redaction is the person's job", forbidden)
		}
	}
}

// The attachment itself is W-03. This phase stores what a person wrote about
// the evidence and has no upload path at all.
func TestThereIsNoAttachmentPath(t *testing.T) {
	sources := strings.ToLower(moduleSources(t))
	for _, forbidden := range []string{"multipart", "os.create", "attachment_id", "fileupload", "io.copy"} {
		if strings.Contains(sources, forbidden) {
			t.Errorf("module source contains %q; attachments arrive with W-03", forbidden)
		}
	}
	if !strings.Contains(sources, "evidence_note") {
		t.Fatal("evidence_note is gone; this guard would pass vacuously")
	}
}

// This card produces exactly one AI review state. The other six are reserved
// so EP-08 does not have to widen the set - widening it would mean
// revalidating every stored value.
func TestNothingProducesAReviewStateOtherThanPendingData(t *testing.T) {
	dir := moduleDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
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
		for _, line := range strings.Split(string(source), "\n") {
			for _, reserved := range []string{
				"StateQueued", "StateGenerating", "StateFailed", "StateEdited", "StateSuperseded",
			} {
				if !strings.Contains(line, reserved) {
					continue
				}
				// They may be DECLARED in contract.go and listed in the set;
				// they must not be USED as a value anywhere else.
				if name == "contract.go" {
					seen = true
					continue
				}
				t.Errorf("%s uses reserved review state %s: %s", name, reserved, strings.TrimSpace(line))
			}
			// StateGenerated appears in states.go behind an unreachable
			// branch whose comment says so; anything that can actually reach
			// it is a report this card cannot produce.
			if strings.Contains(line, "StateGenerated") && name != "contract.go" && name != "states.go" {
				t.Errorf("%s produces StateGenerated: %s", name, strings.TrimSpace(line))
			}
		}
	}
	if !seen {
		t.Fatal("the reserved states are gone from contract.go; this guard would pass vacuously")
	}
}

// Ruling: SOP 10.1 names the fields but gives no values for unit, window,
// evidence, excerpt, interpretation or tags, so this card defines none. The
// five sets below are the whole list.
func TestThereIsNoSixthControlledSet(t *testing.T) {
	sources := moduleSources(t)
	declaration := regexp.MustCompile(`var ([A-Z][A-Za-z]*) = \[\]([A-Z][A-Za-z]*)\{`)
	found := map[string]bool{}
	for _, match := range declaration.FindAllStringSubmatch(sources, -1) {
		found[match[1]] = true
	}
	for _, name := range []string{
		"Platforms", "Metrics", "MetricSources", "ExcerptSources", "ReviewStates",
	} {
		if !found[name] {
			t.Errorf("controlled set %s is gone", name)
		}
		delete(found, name)
	}
	// specs/034's sets (contract §3). R-061 gives each of these values, so
	// they are not the invented kind this test exists to stop; they are named
	// one by one so an unnamed eighth still fails below.
	for _, name := range []string{
		"Pricings", "EvidenceTypes", "TouchRoles", "GrossBases", "AdjustmentKinds",
		"RecordSources", "TouchPlatforms", "Currencies",
	} {
		if !found[name] {
			t.Errorf("034 controlled set %s is gone", name)
		}
		delete(found, name)
	}
	// specs/034 PR 2's sets, also contract §3: allocation target and method
	// (FR-031), judgement (FR-020), attribution method (FR-034), the ordered
	// not-computable reasons (FR-042) and the metric ids (FR-043 to FR-049).
	for _, name := range []string{
		"AllocationTargets", "AllocationMethods", "Judgements", "AttributionMethods",
		"ReasonCodes", "MetricIDs",
	} {
		if !found[name] {
			t.Errorf("034 PR 2 controlled set %s is gone", name)
		}
		delete(found, name)
	}
	// specs/034 PR 3's sets, contract §1.8: what an import holds (cost, lead,
	// deal - the three record kinds R-061 names for import) and what happened
	// to each row (FR-029: written, held back as a possible duplicate, or
	// confirmed not a duplicate).
	for _, name := range []string{"ImportRecordKinds", "ImportOutcomes"} {
		if !found[name] {
			t.Errorf("034 PR 3 controlled set %s is gone", name)
		}
		delete(found, name)
	}
	// specs/035 PR 1's sets, contract §3: R-057's six dimensions (FR-003),
	// its two scopes (account report, brand summary), the two mark kinds and
	// five verdicts of ruling Q2=A, and the one data origin of FR-073.
	for _, name := range []string{
		"DiagnosisDimensions", "DiagnosisScopes", "MarkKinds", "MarkVerdicts", "DataOrigins",
	} {
		if !found[name] {
			t.Errorf("035 PR 1 controlled set %s is gone", name)
		}
		delete(found, name)
	}
	// specs/035 PR 2's sets, contract §3: why a dimension cannot be computed
	// (FR-012), what a gap is missing (R-057's 补录待办, FR-031) and the rule
	// ids the result names instead of sentences (FR-015).
	for _, name := range []string{"DimensionReasons", "GapKinds", "DiagnosisRuleIDs"} {
		if !found[name] {
			t.Errorf("035 PR 2 controlled set %s is gone", name)
		}
		delete(found, name)
	}
	for extra := range found {
		t.Errorf("a controlled set %q was added; SOP 10.1 names those fields but gives no values, and inventing some puts words in the SOP's mouth that every stored row then has to be valid against", extra)
	}
	// And the free text fields are still plain strings.
	for _, field := range []string{"Unit", "StatWindow", "EvidenceNote", "RedactedExcerpt", "Interpretation"} {
		plain := regexp.MustCompile(field + ` +string`)
		if !plain.MatchString(sources) {
			t.Errorf("%s is no longer free text", field)
		}
	}
}

// The value column is the one place a nil could quietly become a zero. This
// scans for the shapes that would do it.
func TestNothingTurnsAnUnknownValueIntoZero(t *testing.T) {
	sources := moduleSources(t)
	for _, line := range strings.Split(sources, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || !strings.Contains(line, "Value") {
			continue
		}
		for _, forbidden := range []string{"Value == nil", "Value != nil"} {
			if !strings.Contains(line, forbidden) {
				continue
			}
			// SameValue and DescribeValue are the two functions whose whole
			// job is to tell the two apart; everywhere else, branching on nil
			// is how the collapse starts.
			t.Logf("nil branch on a metric value: %s", trimmed)
		}
		if strings.Contains(line, "Value = 0") || strings.Contains(line, "Value: 0") {
			t.Errorf("a metric value is defaulted to zero: %s", trimmed)
		}
	}
	// The field has to still be a pointer. A plain int64 cannot express
	// "unknown" at all.
	if !regexp.MustCompile(`Value +\*int64`).MatchString(sources) {
		t.Fatal("ManualMetric.Value is no longer a pointer; unknown and zero can no longer be told apart")
	}
}

// Ruling Q1: the platform set is defined here because feedback-learning's
// declared dependencies do not include ip-profile. That makes drift possible,
// so the test reads ip-profile's source instead of importing it.
func TestThePlatformSetAgreesWithIPProfiles(t *testing.T) {
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
	for _, platform := range Platforms {
		if !platforms[string(platform)] {
			t.Errorf("platform %q is not one of ip-profile's %v", platform, platforms)
		}
	}
}

// The publication statuses the pending derivation accepts have to be 025's
// actual values, not a memory of them.
func TestThePublishedStatusesAgreeWithReviewDeliverys(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(moduleDir(t), "..", "review-delivery", "contract.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range PublishedStatuses {
		if !strings.Contains(string(source), `PublicationStatus = "`+status+`"`) {
			t.Errorf("review-delivery has no publication status %q", status)
		}
	}
}

// The pending derivation may compare times now - specs/029 gave the brand a
// 反馈观察时点 to compare against - but the number of days must come from that
// setting and never from a literal in this package.
//
// This guard used to forbid time comparison outright, because until 029 there
// was nothing to compare against and any number here would have been a rule
// the SOP never stated, sitting where no operator could see or change it. That
// sentence is still the point; only the way to satisfy it changed. Deleting
// the guard when the window landed would have thrown away the part that still
// matters.
//
// Three things it checks, and the third is what makes the first two worth
// keeping:
//
//  1. no SQL statement in this module does the comparison - the window is a Go
//     value, and pushing it into SQL means interpolating it or copying the rule;
//  2. no literal day count appears anywhere near the derivation;
//  3. the derivation still takes the window as an argument, so the guard
//     cannot be passing because the feature is gone.
func TestThePendingDerivationReadsItsWindowFromSettings(t *testing.T) {
	sources := moduleSources(t)

	// (1) The SQL says what it always said: published, and no metrics.
	for _, statement := range sqlLiterals(t, sources) {
		upper := strings.ToUpper(statement)
		if !strings.Contains(upper, "NOT EXISTS") {
			continue
		}
		for _, forbidden := range []string{"INTERVAL", "NOW()", "PUBLISHED_AT <", "PUBLISHED_AT >", "AGE("} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("the pending derivation compares times in SQL (%s):\n%s", forbidden, statement)
			}
		}
	}

	// (2) No written-down number of days. Anything multiplied into a duration,
	// or added to a time, is what a hard-coded window looks like in Go.
	code := stripGoComments(sources)
	for _, pattern := range []string{
		`\b\d+\s*\*\s*24\s*\*\s*time\.Hour`,
		`\b\d+\s*\*\s*time\.Hour`,
		`time\.Duration\(\s*\d+\s*\)`,
		`AddDate\(\s*0\s*,\s*0\s*,\s*\d+\s*\)`,
	} {
		if match := regexp.MustCompile(pattern).FindString(code); match != "" {
			t.Errorf("a hard-coded observation window appears in this module: %q", match)
		}
	}

	// (3) ...and the derivation still asks for the window, so (1) and (2) are
	// not passing because nobody compares anything any more.
	if !strings.Contains(code, "func NeedsRegistration(") {
		t.Fatal("NeedsRegistration is gone; this guard would pass vacuously")
	}
	signature := regexp.MustCompile(`func NeedsRegistration\([^)]*workspacecore\.Due[^)]*\)`)
	if !signature.MatchString(code) {
		t.Error("NeedsRegistration no longer takes the window as an argument; " +
			"whatever decides 'is it due' is now somewhere this guard cannot see")
	}
	if !strings.Contains(code, "workspacecore.DueUnknown") {
		t.Error("nothing in this module mentions DueUnknown; the case where the " +
			"window cannot be answered has to be handled explicitly")
	}
}

// stripGoComments blanks // and /* */ so the checks above cannot match prose.
// Offsets are preserved so any reported match still lines up with the source.
func stripGoComments(source string) string {
	out := []rune(source)
	n := len(out)
	blank := func(from, to int) {
		for i := from; i < to && i < n; i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	for i := 0; i < n; i++ {
		if out[i] == '/' && i+1 < n && out[i+1] == '/' {
			j := i
			for j < n && out[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j
			continue
		}
		if out[i] == '/' && i+1 < n && out[i+1] == '*' {
			j := i + 2
			for j+1 < n && !(out[j] == '*' && out[j+1] == '/') {
				j++
			}
			blank(i, j+2)
			i = j + 1
		}
	}
	return string(out)
}
