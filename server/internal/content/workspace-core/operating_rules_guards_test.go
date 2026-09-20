package workspacecore

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// What this card must NOT be able to do. Every one of these scans the SOURCE,
// not the data: "no platform password was ever saved" is trivially true of an
// empty database, while "no code here can save one" is the claim SOP 3.2
// actually makes ("系统不需要平台登录凭据").
//
// The files this package had before this card - authz.go, grant.go, http.go -
// are scanned too. That is deliberate: a guard that carved out the pre-existing
// files would stop noticing the day somebody added a credential field to one.

func ruleFiles(t *testing.T) []string {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, filepath.Join(dir, name))
	}
	if len(names) == 0 {
		t.Fatal("no package sources found; every guard below would pass vacuously")
	}
	return names
}

func ruleSources(t *testing.T) string {
	t.Helper()
	var combined strings.Builder
	for _, path := range ruleFiles(t) {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		combined.Write(source)
		combined.WriteString("\n")
	}
	return combined.String()
}

// moduleDirFor resolves a sibling content module's directory.
func moduleDirFor(t *testing.T, module string) string {
	t.Helper()
	_, current, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(current), "..", module)
}

// FR-013. The second half of this test is the half that matters: a guard that
// only asserts "no credential fields" is green on a package that stores
// nothing at all. 022 shipped both halves and this copies both.
func TestNoPlatformCredentialIsStored(t *testing.T) {
	sources := ruleSources(t)
	// Comments are stripped first. A guard that matched prose would punish the
	// file for explaining that it stores no credentials, which is backwards.
	lowered := strings.ToLower(stripComments(sources))
	for _, forbidden := range []string{
		"password", "passwd", "app_secret", "appsecret", "client_secret",
		"access_token", "refresh_token", "cookie", "credential", "api_key", "apikey",
	} {
		if strings.Contains(lowered, forbidden) {
			t.Errorf("the package mentions %q; SOP 3.2 says the system needs no platform login", forbidden)
		}
	}
	// ...and it really does write something, so the check above is not vacuous.
	if !strings.Contains(sources, "UPDATE workspace") || !strings.Contains(sources, "UPDATE content_account") {
		t.Fatal("no write path found; the credential guard above would pass on an empty package")
	}
}

// FR-014 / FR-030. The homepage link is stored and never followed.
//
// net/http is not forbidden - http.go has always imported it for status codes,
// and url.Parse in the link check parses rather than fetches. What must not
// exist is anything that CALLS out.
func TestNothingHereReachesOutOfTheProcess(t *testing.T) {
	code := stripComments(ruleSources(t))
	for _, forbidden := range []string{
		"http.Get", "http.Post", "http.Head", "http.Client",
		"http.NewRequest", "http.DefaultClient", "RoundTrip", "net.Dial",
	} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the package can make an outbound request (%s)", forbidden)
		}
	}
	// The link really is handled here, so the checks above are not vacuous.
	if !strings.Contains(code, "ValidateHomepage") {
		t.Fatal("the homepage path is gone; this guard would pass vacuously")
	}
}

// FR-009 / FR-025. "排期用于提醒运营者手动处理" - and this card does not even
// do the reminding: there is no notification system to hang one on.
func TestNothingHereSchedulesOrNotifies(t *testing.T) {
	sources := ruleSources(t)
	code := stripComments(sources)
	for _, forbidden := range []string{
		"time.Ticker", "time.NewTimer", "time.AfterFunc", "cron", "go func()",
		"Notify", "SendMail", "Webhook",
	} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the package looks like it schedules or notifies (%s)", forbidden)
		}
	}
}

// FR-019. Team review is later work. A disabled control is still a promise, so
// there must be nothing here for one to read.
func TestNothingHereSelectsMembers(t *testing.T) {
	sources := ruleSources(t)
	// grant.go legitimately decides over a subject that was handed in; what
	// must not exist is a path that goes looking for the members of a brand in
	// order to assign a reviewer.
	code := stripComments(sources)
	for _, forbidden := range []string{"FROM member", "ListMembers", "reviewer_id", "reviewers"} {
		if strings.Contains(code, forbidden) {
			t.Errorf("the package looks like it picks reviewers (%s)", forbidden)
		}
	}
}

// FR-024 / SC-009. 027 declined to write a default number of days because "a
// hard-coded number of days would be a rule the SOP never stated, sitting in
// code where no operator can see or change it". This card supplies the setting
// and inherits the prohibition.
func TestNoDefaultObservationWindowIsWrittenDown(t *testing.T) {
	for _, path := range ruleFiles(t) {
		name := filepath.Base(path)
		if name != "observation.go" && name != "operating_rules.go" && name != "store.go" {
			continue
		}
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		code := stripComments(string(source))
		// A bare integer literal multiplied into a duration, or compared
		// against a day count, is what a hard-coded window looks like.
		literal := regexp.MustCompile(`\b(7|14|30|90)\b\s*\*`)
		if match := literal.FindString(code); match != "" {
			t.Errorf("%s contains what looks like a hard-coded window: %q", name, strings.TrimSpace(match))
		}
	}
	// And the function that would have used one still exists, so this is not
	// passing because the feature is gone.
	if !strings.Contains(ruleSources(t), "func ObservationDue(") {
		t.Fatal("ObservationDue is gone; this guard would pass vacuously")
	}
}

// FR-012 / SC-016. This package cannot import ip-profile - its declared
// dependency is diagnostics and nothing else - so the set is compared against
// ip-profile's SOURCE, the way review-delivery and feedback-learning compare
// theirs.
func TestThePlatformSetMatchesIPProfiles(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(moduleDirFor(t, "ip-profile"), "account.go"))
	if err != nil {
		t.Fatal(err)
	}
	declaration := regexp.MustCompile(`Platform[A-Za-z]* +Platform += +"([a-z_]+)"`)
	theirs := map[string]bool{}
	for _, match := range declaration.FindAllStringSubmatch(string(source), -1) {
		theirs[match[1]] = true
	}
	if len(theirs) == 0 {
		t.Fatal("read no platforms out of ip-profile; the comparison below would pass vacuously")
	}
	// Equality, not subset: the ruling (Q2=A) put a template on every channel
	// an account can exist on, so a platform ip-profile gains and this set does
	// not is a channel nobody can write a template for.
	if len(theirs) != len(Platforms) {
		t.Errorf("ip-profile has %d platforms, this package has %d", len(theirs), len(Platforms))
	}
	for _, platform := range Platforms {
		if !theirs[platform] {
			t.Errorf("platform %q is not one of ip-profile's %v", platform, theirs)
		}
	}
}

// A write must never replace the settings blob: doing so would delete the
// brand's timezone (LT-009) and precheck switch (019) as a side effect of
// saving a cadence.
func TestWritesSetOneKeyRatherThanReplacingSettings(t *testing.T) {
	sources := stripComments(ruleSources(t))

	// EVERY assignment to the settings column, checked one at a time. The
	// first version of this test asked whether jsonb_set appeared anywhere in
	// the package, and a mutation that changed one of the two statements to
	// jsonb_build_object walked straight past it: the other statement still
	// had a jsonb_set, so the package-wide search was satisfied. The real-DB
	// test caught that mutation; this one did not, which is what sent it back
	// here.
	assignments := regexp.MustCompile(`(?s)SET\s+settings\s*=\s*(.*?),\s*\n`).FindAllStringSubmatch(sources, -1)
	if len(assignments) == 0 {
		t.Fatal("no settings assignment found; this guard would pass vacuously")
	}
	for _, assignment := range assignments {
		expression := strings.TrimSpace(assignment[1])
		if !strings.HasPrefix(expression, "jsonb_set(") {
			t.Errorf("a settings assignment replaces more than one key: %s", expression)
		}
	}
	// Two writes today: the brand's rules and an account's homepage. A third
	// appearing without a jsonb_set is what the loop above is for; a count
	// that drops to one means a write was removed rather than made safe.
	if len(assignments) < 2 {
		t.Errorf("found %d settings assignments, want at least 2 (rules and homepage)", len(assignments))
	}
}

// stripComments blanks // and /* */ so a guard cannot match prose. Keeps
// offsets so line numbers in any failure still line up.
func stripComments(source string) string {
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
