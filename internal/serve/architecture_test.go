package serve

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestServeStorageArchitectureBoundaries prevents handlers and jobs from
// casually creating persistent/temp filesystem state outside the small set of
// storage/cache/staging adapters that intentionally own that responsibility.
// This keeps protected-mode plaintext persistence difficult to introduce by
// accident while leaving ordinary read-only filesystem operations alone.
func TestServeStorageArchitectureBoundaries(t *testing.T) {
	allowedFilesystemMutators := map[string]bool{}

	mutatingOSCalls := map[string]bool{
		"Create":     true,
		"CreateTemp": true,
		"OpenFile":   true,
		"WriteFile":  true,
		"Mkdir":      true,
		"MkdirAll":   true,
		"MkdirTemp":  true,
		"Rename":     true,
		"Remove":     true,
		"RemoveAll":  true,
		"Link":       true,
		"Symlink":    true,
	}

	for _, name := range trackedServeProductionFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports for %s: %v", name, err)
		}

		osAlias := ""
		for _, imp := range file.Imports {
			if imp.Path.Value != `"os"` {
				continue
			}
			osAlias = "os"
			if imp.Name != nil {
				osAlias = imp.Name.Name
			}
			if osAlias == "." {
				t.Run(filepath.Base(name)+"/dot-import-os", func(t *testing.T) {
					t.Fatalf("%s dot-imports os, bypassing the serve storage architecture check", name)
				})
				osAlias = ""
			}
		}
		if osAlias == "" || allowedFilesystemMutators[filepath.Base(name)] {
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
			if !ok || !mutatingOSCalls[sel.Sel.Name] {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != osAlias {
				return true
			}
			pos := fset.Position(call.Pos())
			t.Run(fmt.Sprintf("%s/line-%d/os.%s", filepath.Base(name), pos.Line, sel.Sel.Name), func(t *testing.T) {
				t.Fatalf("%s:%d calls os.%s directly; persistent/temp filesystem mutation belongs in an explicit storage/cache/staging adapter", filepath.ToSlash(name), pos.Line, sel.Sel.Name)
			})
			return true
		})
	}
}

func trackedServeProductionFiles(t *testing.T) []string {
	t.Helper()

	rootOut, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}
	root := strings.TrimSpace(string(rootOut))
	out, err := exec.Command("git", "-C", root, "ls-files", "--", "internal/serve/*.go").Output()
	if err != nil {
		t.Fatalf("list tracked serve files: %v", err)
	}

	var files []string
	for _, tracked := range strings.Fields(string(out)) {
		if filepath.Dir(tracked) != "internal/serve" || strings.HasSuffix(tracked, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(root, tracked))
	}
	if len(files) == 0 {
		t.Fatal("no tracked production serve files found")
	}
	return files
}
