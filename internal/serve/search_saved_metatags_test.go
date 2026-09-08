package serve

import "testing"

func TestMetaTagDTOsExposeCompleteQueryCatalog(t *testing.T) {
	items := metaTagDTOs()
	assertSyntax := func(syntax string, requiresValue bool) {
		t.Helper()
		for _, item := range items {
			if item.Syntax == syntax {
				if item.RequiresValue != requiresValue {
					t.Fatalf("syntax %q requires_value=%v, want %v", syntax, item.RequiresValue, requiresValue)
				}
				return
			}
		}
		t.Fatalf("syntax %q missing from autocomplete catalog", syntax)
	}

	assertSyntax("@tagged", false)
	assertSyntax("@filename_contains:", true)
	assertSyntax("type:", true)
	assertSyntax("type:video", false)
	assertSyntax("ext:", true)
	assertSyntax("ext:jpg", false)
	assertSyntax("ext:mp4", false)
	assertSyntax("ext:cbz", false)
}
