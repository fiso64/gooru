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
// storage/cache/staging functions that intentionally own that responsibility.
// This keeps protected-mode plaintext persistence difficult to introduce by
// accident while leaving read-only, deletion, and relocation operations alone.
func TestServeStorageArchitectureBoundaries(t *testing.T) {
	// Keep this allowlist function-scoped even when the owning file also contains
	// handlers or job orchestration. New direct filesystem creation in those files
	// must still make an explicit architecture decision here.
	allowedFilesystemMutators := map[string]map[string]bool{
		"background_file_removals.go": {
			"(*stagedFileDeletion).stage": true,
		},
		"derivative_store.go": {
			"(*persistentDerivativeStore).GetOrGenerate": true,
			"(*encryptedDerivativeStore).GetOrGenerate":  true,
		},
		"file_delete.go": {
			"stageFileDeletion": true,
		},
		"file_removal_staging_safety.go": {
			"ensureDeletionStagingDirectory": true,
		},
		"media_thumbnailers.go": {
			"commandThumbnailer.Thumbnail": true,
		},
		"opaque_storage.go": {
			"protectedManagedStoragePathForRoot": true,
		},
		"upload_durable_staging.go": {
			"(*Server).stageDurableMultipartUpload": true,
			"prepareDurableUploadStagingDir":        true,
		},
		"upload_stream.go": {
			"(*Server).stageMultipartUpload": true,
			"(*Server).streamUploadPart":     true,
			"moveStreamedUploadIntoDir":      true,
		},
		"uploads.go": {
			"(*Server).saveUploadedFiles": true,
			"createUploadDestination":     true,
		},
	}

	// These are the os primitives that can originate new persistent/plaintext
	// filesystem state. Rename/link/remove are intentionally not included: they
	// manipulate already-created state and have many transactional/recovery uses.
	creatingOSCalls := map[string]bool{
		"Create":     true,
		"CreateTemp": true,
		"OpenFile":   true,
		"WriteFile":  true,
		"Mkdir":      true,
		"MkdirAll":   true,
		"MkdirTemp":  true,
		"Truncate":   true,
	}

	for _, name := range trackedServeProductionFiles(t) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
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
		if osAlias == "" {
			continue
		}

		fileAllowlist := allowedFilesystemMutators[filepath.Base(name)]
		for _, decl := range file.Decls {
			owner := "<package>"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				owner = serveFunctionName(fn)
			}
			allowed := fileAllowlist[owner]
			ast.Inspect(decl, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !creatingOSCalls[sel.Sel.Name] {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != osAlias || allowed {
					return true
				}
				pos := fset.Position(call.Pos())
				t.Run(fmt.Sprintf("%s/%s/line-%d/os.%s", filepath.Base(name), owner, pos.Line, sel.Sel.Name), func(t *testing.T) {
					t.Fatalf("%s:%d %s calls os.%s directly; filesystem creation belongs in an explicitly allowlisted storage/cache/staging function", filepath.ToSlash(name), pos.Line, owner, sel.Sel.Name)
				})
				return true
			})
		}
	}
}

func serveFunctionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	switch recv := fn.Recv.List[0].Type.(type) {
	case *ast.Ident:
		return recv.Name + "." + fn.Name.Name
	case *ast.StarExpr:
		if ident, ok := recv.X.(*ast.Ident); ok {
			return "(*" + ident.Name + ")." + fn.Name.Name
		}
	}
	return "<method>." + fn.Name.Name
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
