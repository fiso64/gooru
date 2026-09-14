package serve

import "testing"

func TestAttachUploadItemTagsPreservesAbsentAndExplicitEmpty(t *testing.T) {
	files := []savedUpload{{name: "one.jpg"}, {name: "two.jpg"}}
	if err := attachUploadItemTags(files, nil); err != nil {
		t.Fatalf("attach absent item tags: %v", err)
	}
	if files[0].tags != nil || files[1].tags != nil {
		t.Fatal("absent item_tags must retain legacy global-tag fallback")
	}
	if err := attachUploadItemTags(files, []string{"shared item:one", ""}); err != nil {
		t.Fatalf("attach aligned item tags: %v", err)
	}
	if files[0].tags == nil || len(*files[0].tags) != 2 || (*files[0].tags)[0] != "shared" || (*files[0].tags)[1] != "item:one" {
		t.Fatalf("first item tags = %#v", files[0].tags)
	}
	if files[1].tags == nil || len(*files[1].tags) != 0 {
		t.Fatalf("explicit empty item tags = %#v; want non-nil empty slice", files[1].tags)
	}
}

func TestAttachUploadItemTagsRejectsMisalignedValues(t *testing.T) {
	files := []savedUpload{{name: "one.jpg"}, {name: "two.jpg"}}
	if err := attachUploadItemTags(files, []string{"only-one"}); err == nil {
		t.Fatal("expected item_tags/file count mismatch to fail")
	}
}
