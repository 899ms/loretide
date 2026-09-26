package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Search adapters may read public work APIs, but must not directly write
// review/delivery/publication state or make outbound requests (T049).
func TestSearchHandlersDoNotWriteDeliveryOrCallNetwork(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(current), "content_search_*.go"))
	if err != nil {
		t.Fatal(err)
	}
	write := regexp.MustCompile(`(?i)\b(?:INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+content_(?:review|delivery|publication)_`)
	checked := 0
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		checked++
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		networkAliases := map[string]bool{}
		for _, spec := range file.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			if imported == "net/http" {
				alias := "http"
				if spec.Name != nil {
					alias = spec.Name.Name
				}
				networkAliases[alias] = true
			} else if imported == "net" || strings.HasPrefix(imported, "net/") {
				t.Errorf("%s imports network package %s", filepath.Base(path), imported)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING {
				value, _ := strconv.Unquote(literal.Value)
				if write.MatchString(value) {
					t.Errorf("%s writes downstream state", filepath.Base(path))
				}
			}
			if selector, ok := node.(*ast.SelectorExpr); ok {
				if pkg, ok := selector.X.(*ast.Ident); ok && networkAliases[pkg.Name] {
					switch selector.Sel.Name {
					case "Get", "Post", "PostForm", "Head", "Client", "DefaultClient", "Transport", "DefaultTransport", "NewRequest", "NewRequestWithContext":
						t.Errorf("%s uses outbound http.%s", filepath.Base(path), selector.Sel.Name)
					}
				}
			}
			return true
		})
	}
	if checked < 2 {
		t.Fatalf("checked only %d search adapters", checked)
	}
}
