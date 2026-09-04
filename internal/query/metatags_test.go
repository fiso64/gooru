package query

import (
	"strings"
	"testing"
)

func TestMetaTagCatalogAndParser(t *testing.T) {
	definitions := MetaTags()
	if len(definitions) != 2 {
		t.Fatalf("got %d metatags, want 2", len(definitions))
	}
	if definitions[0].Name != MetaTagTagged || definitions[1].Name != MetaTagFilenameContains {
		t.Fatalf("unexpected catalog: %+v", definitions)
	}
	parsed, err := ParseMetaTag(`@filename_contains:Summer 100%_Set`)
	if err != nil {
		t.Fatalf("parse filename metatag: %v", err)
	}
	if parsed.Value != `Summer 100%_Set` || !parsed.Definition.RequiresValue {
		t.Fatalf("unexpected parsed metatag: %+v", parsed)
	}
	if _, err := ParseMetaTag(`@filename_contains:`); err == nil {
		t.Fatal("expected empty filename_contains value to fail")
	}
}

func TestFilenameContainsBuildLocationsUsesIndexedBasenameAndLiteralGuard(t *testing.T) {
	expr, err := Parse(`"@filename_contains:100%_SET"`)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAST(expr); err != nil {
		t.Fatal(err)
	}
	queryText, args := BuildLocations(expr, nil)
	if len(args) != 2 || args[0] != `"100%_SET"` || args[1] != `100%_SET` {
		t.Fatalf("unexpected filename_contains args: %#v", args)
	}
	for _, fragment := range []string{
		"FROM location_filenames lf JOIN locations l ON l.id = lf.rowid",
		"lf.filename MATCH ?",
		"instr(lower(lf.filename), lower(?)) > 0",
	} {
		if !strings.Contains(queryText, fragment) {
			t.Fatalf("query missing %q: %s", fragment, queryText)
		}
	}
	for _, stale := range []string{"WITH RECURSIVE", "LIKE"} {
		if strings.Contains(queryText, stale) {
			t.Fatalf("query unexpectedly contains stale %q path: %s", stale, queryText)
		}
	}
}
