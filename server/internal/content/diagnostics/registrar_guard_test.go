package diagnostics

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The host-fact registration methods are exported on the same *Service the
// HTTP handler holds, so "only host wiring can register facts" (spec §5) is a
// convention, not a type boundary. This guard pins the convention: it scans
// every non-test Go file under server/ and fails when any file other than the
// host wiring or the definition file references a registration method.
func TestHostFactRegistrationIsReferencedOnlyFromHostWiring(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	serverDir := filepath.Clean(filepath.Join(filepath.Dir(current), "..", "..", ".."))

	registrars := map[string]bool{
		"RegisterServerBootFacts":         true,
		"RegisterStorageFacts":            true,
		"RegisterExecutionPolicyDisabled": true,
	}
	allowed := map[string]bool{
		"cmd/server/main.go":                      true,
		"cmd/server/router.go":                    true,
		"internal/content/diagnostics/service.go": true,
	}

	found := map[string]map[string]bool{}
	fset := gotoken.NewFileSet()
	err := filepath.WalkDir(serverDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if name := entry.Name(); path != serverDir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Errorf("parse %s: %v", path, parseErr)
			return nil
		}
		rel, _ := filepath.Rel(serverDir, path)
		rel = filepath.ToSlash(rel)
		record := func(name string, pos gotoken.Pos) {
			if !registrars[name] {
				return
			}
			if !allowed[rel] {
				t.Errorf("%s references host-fact registrar %s; only cmd/server/main.go and cmd/server/router.go may register facts", fset.Position(pos), name)
			}
			if found[rel] == nil {
				found[rel] = map[string]bool{}
			}
			found[rel][name] = true
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.SelectorExpr:
				record(n.Sel.Name, n.Sel.Pos())
			case *ast.FuncDecl:
				if n.Recv != nil {
					record(n.Name.Name, n.Name.Pos())
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Without these the guard would pass vacuously after a rename or a move.
	for rel, names := range map[string][]string{
		"cmd/server/main.go":                      {"RegisterServerBootFacts", "RegisterExecutionPolicyDisabled"},
		"cmd/server/router.go":                    {"RegisterStorageFacts"},
		"internal/content/diagnostics/service.go": {"RegisterServerBootFacts", "RegisterStorageFacts", "RegisterExecutionPolicyDisabled"},
	} {
		for _, name := range names {
			if !found[rel][name] {
				t.Errorf("expected %s in %s; the scan did not see it, so this guard may be scanning the wrong tree", name, rel)
			}
		}
	}
}
