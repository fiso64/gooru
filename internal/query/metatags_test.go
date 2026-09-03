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

func TestFilenameContainsBuildLocationsTargetsBasenameCaseInsensitivelyAndEscapesWildcards(t *testing.T) {
	expr, err := Parse(`"@filename_contains:100%_SET"`)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAST(expr); err != nil {
		t.Fatal(err)
	}
	queryText, args := BuildLocations(expr, nil)
	if len(args) != 1 || args[0] != `%100\%\_SET%` {
		t.Fatalf("unexpected LIKE args: %#v", args)
	}
	for _, fragment := range []string{"WITH RECURSIVE path_parts", "replace(path, char(92), '/')", "p.rest = ''", "lower(p.part) LIKE lower(?) ESCAPE '\\'"} {
		if !strings.Contains(queryText, fragment) {
			t.Fatalf("query missing %q: %s", fragment, queryText)
		}
	}
}
