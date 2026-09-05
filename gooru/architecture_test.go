package gooru

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCoreStorageArchitectureBoundaries keeps encryption/storage safety on the
// mandatory path. Business-logic files may discover paths and inspect existence,
// but logical media contents must flow through Client's configured source
// resolver, and database implementation selection belongs to the composition
// layer in gooru.go.
func TestCoreStorageArchitectureBoundaries(t *testing.T) {
	for _, name := range trackedCoreProductionFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", name, err)
		}

		osAlias := ""
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if path == "gooru.local/internal/database" && name != "gooru.go" {
				t.Errorf("%s imports the database implementation directly; keep DB opening/policy in gooru.go", name)
			}
			if path == "os" {
				osAlias = "os"
				if imp.Name != nil {
					osAlias = imp.Name.Name
				}
				if osAlias == "." {
					t.Errorf("%s dot-imports os, bypassing the storage architecture check", name)
					osAlias = ""
				}
			}
		}
		if osAlias == "" {
			continue
		}

		file, err = parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != osAlias {
				return true
			}
			switch sel.Sel.Name {
			case "Open", "OpenFile", "ReadFile", "WriteFile", "Create", "CreateTemp", "MkdirTemp":
				pos := fset.Position(call.Pos())
				t.Errorf("%s:%d calls os.%s directly; logical tracked-media IO and temp/persistent writes must use the source/storage capabilities", filepath.ToSlash(name), pos.Line, sel.Sel.Name)
			}
			return true
		})
	}
}

func trackedCoreProductionFiles(t *testing.T) []string {
	t.Helper()

	rootOut, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	root := strings.TrimSpace(string(rootOut))
	out, err := exec.Command("git", "-C", root, "ls-files", "--", "gooru/*.go").Output()
	if err != nil {
		t.Fatalf("list tracked core files: %v", err)
	}

	var files []string
	for _, tracked := range strings.Fields(string(out)) {
		if filepath.Dir(tracked) != "gooru" || strings.HasSuffix(tracked, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(root, tracked))
	}
	if len(files) == 0 {
		t.Fatal("no tracked production core files found")
	}
	return files
}
