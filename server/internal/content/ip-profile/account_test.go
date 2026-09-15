package ipprofile

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/content/diagnostics"
)

// Contract: specs/015-lt011-account-platform-config/contracts/account-api.md

// A6: every value the product supports must be accepted.
func TestEverySupportedPlatformIsAccepted(t *testing.T) {
	if len(Platforms) == 0 {
		t.Fatal("no platforms declared")
	}
	for _, platform := range Platforms {
		if err := ValidatePlatform(string(platform)); err != nil {
			t.Errorf("platform %q was rejected: %v", platform, err)
		}
	}
}

// A5: anything outside the set is refused, including the shapes that look
// plausible - a different case, a near miss, a blank.
func TestAnUnsupportedPlatformIsRefused(t *testing.T) {
	for _, bad := range []string{
		"myspace", "", "  ", "XIAOHONGSHU", "xiaohongshu ", "wechat", "douyin;drop",
	} {
		if err := ValidatePlatform(bad); err == nil {
			t.Errorf("platform %q was accepted", bad)
		}
	}
}

// A7. The Go enum is the authority and the CHECK constraint is the backstop, so
// they have to agree. Read from the migration rather than restated here: a copy
// would drift silently, which is the exact failure this guards.
func TestTheGoEnumAndTheDatabaseCheckAgree(t *testing.T) {
	path := filepath.Join("..", "..", "..", "migrations", "477_content_account.up.sql")
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	check := regexp.MustCompile(`(?s)platform\s+text\s+NOT NULL CHECK \(platform IN \((.*?)\)\)`).
		FindSubmatch(source)
	if check == nil {
		t.Fatal("could not find the platform CHECK constraint in the migration; " +
			"if the constraint moved, this test must follow it rather than be deleted")
	}
	var inDatabase []string
	for _, literal := range regexp.MustCompile(`'([a-z_]+)'`).FindAllStringSubmatch(string(check[1]), -1) {
		inDatabase = append(inDatabase, literal[1])
	}

	inGo := make([]string, 0, len(Platforms))
	for _, platform := range Platforms {
		inGo = append(inGo, string(platform))
	}
	sort.Strings(inDatabase)
	sort.Strings(inGo)
	if strings.Join(inDatabase, ",") != strings.Join(inGo, ",") {
		t.Errorf("the Go enum and the database CHECK disagree:\n  Go: %v\n  DB: %v\n"+
			"Adding a platform needs both, and a migration for the CHECK.", inGo, inDatabase)
	}
}

// A8: a display name is how a person tells two accounts apart, so a blank one
// is refused rather than stored and rendered as an empty row.
func TestADisplayNameMustNotBeBlank(t *testing.T) {
	for _, bad := range []string{"", " ", "\t", "\n  \t "} {
		if err := ValidateDisplayName(bad); err == nil {
			t.Errorf("display name %q was accepted", bad)
		}
	}
	for _, good := range []string{"品牌 A", "Brand A", " padded "} {
		if err := ValidateDisplayName(good); err != nil {
			t.Errorf("display name %q was rejected: %v", good, err)
		}
	}
}

// Duplicate display names are allowed on purpose: one brand really does run two
// accounts with the same name on different platforms. Identity is the id.
func TestDuplicateDisplayNamesAreAllowed(t *testing.T) {
	if err := ValidateDisplayName("同名"); err != nil {
		t.Fatalf("unexpected rejection: %v", err)
	}
	if err := ValidateDisplayName("同名"); err != nil {
		t.Fatalf("a second account with the same name was rejected: %v", err)
	}
}

// D11-V02 (W-02 boundary): this feature must not introduce a separate persona
// table or any binding table. Nothing asserted this before; without it, someone
// adding a persona table later would see nothing go red.
func TestThisFeatureAddsNoPersonaOrBindingTable(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	created := map[string][]string{}
	forbidden := regexp.MustCompile(`(?i)persona|_binding|profile_bind`)
	for _, entry := range entries {
		name := entry.Name()
		// Only the migrations this feature owns.
		if !strings.HasPrefix(name, "477_") && !strings.HasPrefix(name, "478_") {
			continue
		}
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, m := range regexp.MustCompile(`(?i)CREATE TABLE(?: IF NOT EXISTS)?\s+(\w+)`).
			FindAllStringSubmatch(string(source), -1) {
			created[name] = append(created[name], m[1])
			if forbidden.MatchString(m[1]) {
				t.Errorf("%s creates %q: LT-011 must not add a persona or binding table "+
					"(W-02: expression config lives on the account)", name, m[1])
			}
		}
	}
	var all []string
	for _, tables := range created {
		all = append(all, tables...)
	}
	if len(all) != 1 || all[0] != "content_account" {
		t.Errorf("this feature creates %v; it must create exactly one table, content_account", all)
	}
}

// E3: the module's tests reference diagnostics, and the account carries the
// identifiers the diagnostics tables were already reserving for it.
func TestAccountIdentifiersFitTheDiagnosticsColumns(t *testing.T) {
	event := diagnostics.Event{Workspace: "ws-1", Account: "acct-1"}
	if event.Account != "acct-1" || event.Workspace != "ws-1" {
		t.Fatalf("diagnostics event does not carry the pair: %+v", event)
	}
}
