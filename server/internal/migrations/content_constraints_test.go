package migrations

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Feature 010 (specs/010-diag-evidence-gaps) FR / SC map:
//
//	FR-006  the diagnostics module's migrations carry no foreign key, no
//	        cascade, and every index is created CONCURRENTLY in a file of its
//	        own                                                      (SC-004)
//
// Feature 022 extends that policy from migration 483 onward with R5: PRIMARY
// KEY and UNIQUE constraints are also indexes and therefore cannot be hidden
// inside a table migration (FR-013 / SC-008).
//
// This closes the DIAG-04 row of docs/development/diagnostics-acceptance-mapping.md,
// which #48 had just recorded as verified BY HAND ("仓库迁移测试仍不检查这两项,
// 属人工核对"). A hand check is true on the day it is done; this one is true on
// every run.
//
// Deliberately text-only, no database. The constraints are properties of the
// migration FILE: an index that already exists gives no sign of whether it was
// built concurrently, so applying the migrations would test something else.

// contentMigrationFloor is the first migration that belongs to the diagnostics
// module. Without a floor this would sweep in 1000+ historical migrations and
// turn every other module's foreign key into a red line owned by this one.
const contentMigrationFloor = 468

// implicitIndexRuleFloor is the first topic-planning migration. Migrations 477
// and 479 predate the rule and contain inline primary keys; changing them would
// be a separate production migration, not a lint repair. New content migrations
// must keep every index in its own concurrent migration.
const implicitIndexRuleFloor = 483

// violation is one broken rule, addressed to whoever has to fix it.
type violation struct {
	file string
	rule string
	why  string
}

func (v violation) String() string { return fmt.Sprintf("%s: %s %s", v.file, v.rule, v.why) }

var (
	reForeignKey        = regexp.MustCompile(`(?i)\b(REFERENCES|FOREIGN\s+KEY)\b`)
	reCascade           = regexp.MustCompile(`(?i)\bCASCADE\b`)
	reCreateIndex       = regexp.MustCompile(`(?i)\bCREATE\s+(UNIQUE\s+)?INDEX\b`)
	reCreateUniqueIndex = regexp.MustCompile(`(?i)\bCREATE\s+UNIQUE\s+INDEX\b`)
	reConcurrent        = regexp.MustCompile(`(?i)\bCREATE\s+(UNIQUE\s+)?INDEX\s+CONCURRENTLY\b`)
	rePrimaryKey        = regexp.MustCompile(`(?i)\bPRIMARY\s+KEY\b`)
	reUnique            = regexp.MustCompile(`(?i)\bUNIQUE\b`)
	reNumber            = regexp.MustCompile(`^(\d+)_`)
)

// stripSQLTrivia blanks out line comments, block comments and string literals,
// keeping offsets so the result still lines up with the source.
//
// Without this a migration whose comment reads "-- deliberately no FOREIGN KEY"
// would be reported as carrying a foreign key — the comment says the opposite of
// what the match claims. Counting statements has the same problem: a semicolon
// inside a string literal is not a statement boundary.
func stripSQLTrivia(sql string) string {
	out := []rune(sql)
	runes := []rune(sql)
	n := len(runes)
	blank := func(from, to int) {
		for k := from; k < to && k < n; k++ {
			if out[k] != '\n' {
				out[k] = ' '
			}
		}
	}
	for i := 0; i < n; {
		switch {
		case runes[i] == '-' && i+1 < n && runes[i+1] == '-':
			j := i
			for j < n && runes[j] != '\n' {
				j++
			}
			blank(i, j)
			i = j
		case runes[i] == '/' && i+1 < n && runes[i+1] == '*':
			j := i + 2
			for j+1 < n && !(runes[j] == '*' && runes[j+1] == '/') {
				j++
			}
			blank(i, min(j+2, n))
			i = j + 2
		case runes[i] == '\'' || runes[i] == '"':
			quote := runes[i]
			j := i + 1
			for j < n && runes[j] != quote {
				j++
			}
			blank(i, min(j+1, n))
			i = j + 1
		default:
			i++
		}
	}
	return string(out)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// checkContentMigrations is the whole rule set, over a name -> contents map.
//
// A map rather than a directory so the negative cases below are synthetic. A
// fixture written into server/migrations/ would be picked up by
// TestMigrationNumericPrefixesAreUnique and TestMigrationFilesHaveMatchingDirections,
// and the two checks would fight each other.
func checkContentMigrations(files map[string]string) []violation {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	found := []violation{}
	for _, name := range names {
		if !isContentMigration(name) {
			continue
		}
		sql := stripSQLTrivia(files[name])

		if m := reForeignKey.FindString(sql); m != "" {
			found = append(found, violation{name, "R1", fmt.Sprintf("foreign key reference is not allowed (matched %q)", strings.TrimSpace(m))})
		}
		if m := reCascade.FindString(sql); m != "" {
			found = append(found, violation{name, "R2", fmt.Sprintf("cascade is not allowed (matched %q)", strings.TrimSpace(m))})
		}
		indexes := reCreateIndex.FindAllString(sql, -1)
		concurrent := reConcurrent.FindAllString(sql, -1)
		if len(indexes) != len(concurrent) {
			found = append(found, violation{name, "R3", fmt.Sprintf("index must be created CONCURRENTLY (%d index statements, %d concurrent)", len(indexes), len(concurrent))})
		}
		// PRIMARY KEY and UNIQUE constraints also build indexes, but PostgreSQL
		// does not offer a concurrent form for those implicit builds. Explicit
		// CREATE UNIQUE INDEX statements account for one UNIQUE token each and
		// are already governed by R3/R4.
		number, _ := strconv.Atoi(reNumber.FindStringSubmatch(name)[1])
		if number >= implicitIndexRuleFloor {
			primaryKeys := len(rePrimaryKey.FindAllString(sql, -1))
			uniqueConstraints := len(reUnique.FindAllString(sql, -1)) - len(reCreateUniqueIndex.FindAllString(sql, -1))
			if primaryKeys > 0 || uniqueConstraints > 0 {
				found = append(found, violation{name, "R5", fmt.Sprintf("PRIMARY KEY/UNIQUE constraint creates a non-concurrent index (%d primary key, %d unique constraints)", primaryKeys, uniqueConstraints)})
			}
		}
		// PostgreSQL refuses a concurrent index build inside a transaction or a
		// multi-statement string, so a file that builds one may hold nothing
		// else.
		if len(concurrent) > 0 {
			if stmts := countStatements(sql); stmts != 1 {
				found = append(found, violation{name, "R4", fmt.Sprintf("a concurrent index migration must be a single statement (found %d)", stmts)})
			}
		}
	}
	return found
}

// isContentMigration selects the diagnostics module's migrations: numbered at or
// above the floor, and named with the module's content_ prefix.
func isContentMigration(name string) bool {
	m := reNumber.FindStringSubmatch(name)
	if m == nil {
		return false
	}
	number, err := strconv.Atoi(m[1])
	if err != nil || number < contentMigrationFloor {
		return false
	}
	return strings.Contains(name, "content_") && strings.HasSuffix(name, ".sql")
}

func countStatements(sql string) int {
	n := 0
	for _, part := range strings.Split(sql, ";") {
		if strings.TrimSpace(part) != "" {
			n++
		}
	}
	return n
}

// TestContentMigrationConstraints holds the repository's own migrations to the
// rules. This is the assertion that replaces the hand check.
func TestContentMigrationConstraints(t *testing.T) {
	// realMigrationsDir is the locator the other two migration lints already
	// use; ResolveDir walks up from the working directory and lands on this
	// package rather than server/migrations when run from here.
	dir := realMigrationsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	files := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() || !isContentMigration(entry.Name()) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = string(b)
	}
	if len(files) == 0 {
		// A check that finds nothing to check is green for the wrong reason.
		t.Fatalf("no content_ migrations at or above %d were found in %s", contentMigrationFloor, dir)
	}
	for _, v := range checkContentMigrations(files) {
		t.Errorf("%s", v)
	}
	t.Logf("checked %d content_ migration files at or above %d", len(files), contentMigrationFloor)
}

// TestContentMigrationConstraintsCatchViolations is the other half: the rules
// have to reject what they are there to reject. Every fixture is synthetic.
func TestContentMigrationConstraintsCatchViolations(t *testing.T) {
	cases := []struct {
		name string
		file string
		sql  string
		rule string
	}{
		{
			name: "foreign key",
			file: "480_content_thing.up.sql",
			sql:  "CREATE TABLE content_thing (id text NOT NULL, owner uuid REFERENCES workspaces(id));",
			rule: "R1",
		},
		{
			name: "cascade",
			file: "481_content_thing.up.sql",
			sql:  "CREATE TABLE content_thing (id text NOT NULL, owner uuid, CONSTRAINT fk FOREIGN KEY (owner) REFERENCES workspaces(id) ON DELETE CASCADE);",
			rule: "R2",
		},
		{
			name: "index without CONCURRENTLY",
			file: "482_content_thing_idx.up.sql",
			sql:  "CREATE INDEX IF NOT EXISTS content_thing_idx ON content_thing (id);",
			rule: "R3",
		},
		{
			name: "concurrent index sharing a file",
			file: "483_content_thing_idx.up.sql",
			sql:  "CREATE TABLE content_other (id text);\nCREATE INDEX CONCURRENTLY content_thing_idx ON content_thing (id);",
			rule: "R4",
		},
		{
			name: "inline primary key",
			file: "484_content_thing.up.sql",
			sql:  "CREATE TABLE content_thing (id text PRIMARY KEY);",
			rule: "R5",
		},
		{
			name: "inline unique constraint",
			file: "485_content_thing.up.sql",
			sql:  "CREATE TABLE content_thing (id text NOT NULL, slug text UNIQUE);",
			rule: "R5",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := checkContentMigrations(map[string]string{tc.file: tc.sql})
			if len(found) == 0 {
				t.Fatalf("no violation reported for %s", tc.name)
			}
			hit := false
			for _, v := range found {
				if v.rule == tc.rule {
					hit = true
					if !strings.Contains(v.String(), tc.file) {
						t.Errorf("violation does not name the file: %s", v)
					}
				}
			}
			if !hit {
				t.Fatalf("expected %s, got %v", tc.rule, found)
			}
		})
	}
}

// A comment saying a constraint was deliberately avoided must not be read as the
// constraint itself. This is the case a plain text match gets wrong, and it gets
// it wrong in the direction that punishes the author for documenting the rule.
func TestContentMigrationConstraintsIgnoreCommentsAndStrings(t *testing.T) {
	files := map[string]string{
		"484_content_thing.up.sql": "-- deliberately no FOREIGN KEY here, see constitution principle V\n" +
			"/* nor any CASCADE */\n" +
			"CREATE TABLE content_thing (id text NOT NULL, note text NOT NULL DEFAULT 'REFERENCES nothing; CASCADE nothing');",
	}
	if found := checkContentMigrations(files); len(found) != 0 {
		t.Fatalf("comments and string literals were read as SQL: %v", found)
	}
}

// Migrations below the floor, and migrations of other modules, are none of this
// check's business.
func TestContentMigrationConstraintsScopeExcludesOtherModules(t *testing.T) {
	files := map[string]string{
		"415_seat_capacity_outbox.up.sql": "CREATE TABLE seat_capacity_outbox (id uuid REFERENCES workspaces(id) ON DELETE CASCADE);",
		"100_content_legacy.up.sql":       "CREATE TABLE content_legacy (id uuid REFERENCES workspaces(id));",
	}
	if found := checkContentMigrations(files); len(found) != 0 {
		t.Fatalf("check reached outside the diagnostics module: %v", found)
	}
}
