package migrations

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

// R6: every content migration that builds an index CONCURRENTLY on up must be
// registered in cmd/migrate's concurrentIndexCleanups.
//
// The hazard is upstream MUL-5999's: an interrupted `CREATE INDEX CONCURRENTLY`
// leaves an INVALID relation behind. On retry `IF NOT EXISTS` sees it, reports
// success, and the runner records the migration as applied - so the index stays
// permanently unusable and nothing at runtime says so. The registration lets the
// migrator drop the leftover before retrying.
//
// Upstream already checks this in cmd/migrate (TestEveryConcurrentUpBuildHasCleanup),
// and that check failed on app-main from the first content index migration until
// Issue #122, unnoticed for fifteen migrations: ci.yml runs only on `main`, and
// loretide-content.yml named a handful of tests in this package and none in
// cmd/migrate. The rule is repeated here because this is the file a content
// migration author is pointed at, and because this package IS on the branch's
// CI path.
//
// Reading the registration out of the upstream source rather than importing it
// is forced - it lives in `package main`. Same shape as the TypeScript constant
// tests that read their Go source: compare against the real thing, so a change
// on either side is visible here.

// concurrentIndexRegistryVar is the upstream map this rule reads.
const concurrentIndexRegistryVar = "concurrentIndexCleanups"

// reConcurrentIndexName pulls the index name out of a concurrent build. Kept
// separate from reConcurrent above, which only has to decide whether one is
// present.
var reConcurrentIndexName = regexp.MustCompile(
	`(?i)CREATE\s+(?:UNIQUE\s+)?INDEX\s+CONCURRENTLY\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z0-9_]+)`)

// parseConcurrentIndexCleanups reads the registration map out of
// server/cmd/migrate/main.go.
//
// Parsed as Go rather than matched with a regex: a commented-out entry or a
// string containing a brace would both fool a text match, and this map is the
// thing the rule trusts. An entry it cannot read as a plain string literal is
// an error, not a skip.
func parseConcurrentIndexCleanups(path string) (map[string]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range valueSpec.Names {
				if name.Name != concurrentIndexRegistryVar || i >= len(valueSpec.Values) {
					continue
				}
				literal, ok := valueSpec.Values[i].(*ast.CompositeLit)
				if !ok {
					return nil, fmt.Errorf("%s is not a composite literal", concurrentIndexRegistryVar)
				}
				return entriesOf(literal)
			}
		}
	}
	return nil, fmt.Errorf("%s not found in %s", concurrentIndexRegistryVar, path)
}

func entriesOf(literal *ast.CompositeLit) (map[string]string, error) {
	entries := make(map[string]string, len(literal.Elts))
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return nil, fmt.Errorf("%s has a non key-value element", concurrentIndexRegistryVar)
		}
		key, err := stringLiteral(pair.Key)
		if err != nil {
			return nil, err
		}
		value, err := stringLiteral(pair.Value)
		if err != nil {
			return nil, err
		}
		entries[key] = value
	}
	return entries, nil
}

func stringLiteral(expr ast.Expr) (string, error) {
	basic, ok := expr.(*ast.BasicLit)
	if !ok || basic.Kind != token.STRING {
		return "", fmt.Errorf("%s has a non-literal entry", concurrentIndexRegistryVar)
	}
	return strconv.Unquote(basic.Value)
}

// checkContentIndexRegistration is R6 over a name -> contents map, against a
// registration map. Down migrations are out of scope: their counterpart rule
// (concurrentDownIndexCleanups) covers rebuilds on rollback, and the content
// module's down files only drop.
func checkContentIndexRegistration(files map[string]string, registry map[string]string) []violation {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	found := []violation{}
	for _, name := range names {
		if !isContentMigration(name) || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		match := reConcurrentIndexName.FindStringSubmatch(stripSQLTrivia(files[name]))
		if match == nil {
			continue
		}
		version := strings.TrimSuffix(name, ".up.sql")
		index := match[1]
		registered, ok := registry[version]
		if !ok {
			found = append(found, violation{name, "R6", fmt.Sprintf(
				"builds %q concurrently but is not registered in cmd/migrate's %s; an interrupted build would be recorded as success on retry",
				index, concurrentIndexRegistryVar)})
			continue
		}
		if registered != index {
			found = append(found, violation{name, "R6", fmt.Sprintf(
				"%s registers %q but the migration builds %q; a hook naming an index nothing creates is a silent no-op",
				concurrentIndexRegistryVar, registered, index)})
		}
	}
	return found
}

// migrateMainPath locates the upstream file holding the registration map.
func migrateMainPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(realMigrationsDir(t), "..", "cmd", "migrate", "main.go")
}

// TestContentConcurrentIndexRegistration holds the repository's own content
// migrations to R6.
func TestContentConcurrentIndexRegistration(t *testing.T) {
	registry, err := parseConcurrentIndexCleanups(migrateMainPath(t))
	if err != nil {
		t.Fatalf("read registration map: %v", err)
	}
	// A registry that came back empty would make every migration a violation,
	// which is loud; one that came back with a handful of entries because the
	// parse went wrong would not be. Upstream's own map is long.
	if len(registry) < 100 {
		t.Fatalf("read only %d entries from %s; the parse is wrong", len(registry), concurrentIndexRegistryVar)
	}

	dir := realMigrationsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	files := map[string]string{}
	concurrent := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isContentMigration(name) || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		files[name] = string(b)
		if reConcurrentIndexName.MatchString(stripSQLTrivia(files[name])) {
			concurrent++
		}
	}
	if concurrent == 0 {
		// Every content index migration is one concurrent build in a file of
		// its own (R3/R4), so finding none means the selector is broken, not
		// that the module stopped building indexes.
		t.Fatalf("no content_ up migration at or above %d builds an index concurrently", contentMigrationFloor)
	}
	for _, v := range checkContentIndexRegistration(files, registry) {
		t.Errorf("%s", v)
	}
	t.Logf("checked %d concurrent content index migrations against %d registered entries", concurrent, len(registry))
}

// TestContentConcurrentIndexRegistrationCatchesViolations is the other half.
func TestContentConcurrentIndexRegistrationCatchesViolations(t *testing.T) {
	registry := map[string]string{"490_content_thing_idx": "content_thing_idx"}
	cases := []struct {
		name string
		file string
		sql  string
		want string
	}{
		{
			name: "unregistered",
			file: "491_content_other_idx.up.sql",
			sql:  "CREATE INDEX CONCURRENTLY IF NOT EXISTS content_other_idx ON content_other (id);",
			want: "not registered",
		},
		{
			name: "registered under the wrong index name",
			file: "490_content_thing_idx.up.sql",
			sql:  "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS content_thing_renamed_idx ON content_thing (id);",
			want: "silent no-op",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			found := checkContentIndexRegistration(map[string]string{tc.file: tc.sql}, registry)
			if len(found) != 1 {
				t.Fatalf("want one R6 violation, got %v", found)
			}
			if found[0].rule != "R6" {
				t.Errorf("want R6, got %s", found[0].rule)
			}
			if !strings.Contains(found[0].String(), tc.file) {
				t.Errorf("violation does not name the file: %s", found[0])
			}
			if !strings.Contains(found[0].String(), tc.want) {
				t.Errorf("violation does not say %q: %s", tc.want, found[0])
			}
		})
	}

	t.Run("a registered migration passes", func(t *testing.T) {
		found := checkContentIndexRegistration(map[string]string{
			"490_content_thing_idx.up.sql": "CREATE INDEX CONCURRENTLY IF NOT EXISTS content_thing_idx ON content_thing (id);",
		}, registry)
		if len(found) != 0 {
			t.Fatalf("registered migration reported: %v", found)
		}
	})

	t.Run("a down migration is out of scope", func(t *testing.T) {
		found := checkContentIndexRegistration(map[string]string{
			"491_content_other_idx.down.sql": "DROP INDEX CONCURRENTLY IF EXISTS content_other_idx;",
		}, registry)
		if len(found) != 0 {
			t.Fatalf("down migration reported: %v", found)
		}
	})

	t.Run("a comment mentioning a concurrent build is not one", func(t *testing.T) {
		found := checkContentIndexRegistration(map[string]string{
			"491_content_other.up.sql": "-- CREATE INDEX CONCURRENTLY content_other_idx would go here\nCREATE TABLE content_other (id text NOT NULL);",
		}, registry)
		if len(found) != 0 {
			t.Fatalf("comment reported as a build: %v", found)
		}
	})
}
