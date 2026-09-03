package serve

import "testing"

func TestMetaTagDTOsExposeCompleteQueryCatalog(t *testing.T) {
	items := metaTagDTOs()
	if len(items) != 2 {
		t.Fatalf("got %d metatags, want 2", len(items))
	}
	if items[0].Syntax != "@tagged" || items[0].RequiresValue {
		t.Fatalf("unexpected tagged metadata: %+v", items[0])
	}
	if items[1].Syntax != "@filename_contains:" || !items[1].RequiresValue {
		t.Fatalf("unexpected filename metadata: %+v", items[1])
	}
}
