package gooru

import (
	"testing"

	"gooru.local/types"
)

func TestTagKnownFilesWithBackgroundTasksByFileTagsAppliesAlignedTagsAtomically(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	files := []types.LocationInfo{
		{Path: "/library/one.jpg", Hash: "hash-per-file-one", Size: 11, ModTime: 21, Extension: ".jpg"},
		{Path: "/library/two.jpg", Hash: "hash-per-file-two", Size: 12, ModTime: 22, Extension: ".jpg"},
	}
	result, err := client.TagKnownFilesWithBackgroundTasksByFileTags(files, [][]string{{"shared", "item:one"}, {"shared", "item:two"}}, nil, nil)
	if err != nil {
		t.Fatalf("TagKnownFilesWithBackgroundTasksByFileTags: %v", err)
	}
	if result.AffectedCount != 4 {
		t.Fatalf("affected count = %d, want 4", result.AffectedCount)
	}
	oneTags, err := client.store.GetTagsForContent(files[0].Hash)
	if err != nil {
		t.Fatalf("GetTagsForContent(one): %v", err)
	}
	twoTags, err := client.store.GetTagsForContent(files[1].Hash)
	if err != nil {
		t.Fatalf("GetTagsForContent(two): %v", err)
	}
	if !sameStringSet(oneTags, []string{"shared", "item:one"}) {
		t.Fatalf("one tags = %#v", oneTags)
	}
	if !sameStringSet(twoTags, []string{"shared", "item:two"}) {
		t.Fatalf("two tags = %#v", twoTags)
	}
}

func TestTagExistingContentByHashTagsAppliesDistinctTagsInOneBatch(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	files := []types.LocationInfo{
		{Path: "/library/existing-one.jpg", Hash: "hash-existing-one", Size: 11, ModTime: 21, Extension: ".jpg"},
		{Path: "/library/existing-two.jpg", Hash: "hash-existing-two", Size: 12, ModTime: 22, Extension: ".jpg"},
	}
	if _, err := client.TagKnownFilesWithBackgroundTasksByFileTags(files, [][]string{{"seed"}, {"seed"}}, nil, nil); err != nil {
		t.Fatalf("seed existing content: %v", err)
	}
	result, err := client.TagExistingContentByHashTags(
		[]string{files[0].Hash, files[1].Hash, files[0].Hash},
		[][]string{{"shared", "item:one"}, {"shared", "item:two"}, {"later"}},
	)
	if err != nil {
		t.Fatalf("TagExistingContentByHashTags: %v", err)
	}
	if result.AffectedCount != 5 {
		t.Fatalf("affected count = %d, want 5", result.AffectedCount)
	}
	oneTags, err := client.store.GetTagsForContent(files[0].Hash)
	if err != nil {
		t.Fatalf("GetTagsForContent(one): %v", err)
	}
	twoTags, err := client.store.GetTagsForContent(files[1].Hash)
	if err != nil {
		t.Fatalf("GetTagsForContent(two): %v", err)
	}
	if !sameStringSet(oneTags, []string{"seed", "shared", "item:one", "later"}) {
		t.Fatalf("one tags = %#v", oneTags)
	}
	if !sameStringSet(twoTags, []string{"seed", "shared", "item:two"}) {
		t.Fatalf("two tags = %#v", twoTags)
	}
}

func TestTagKnownFilesWithBackgroundTasksByFileTagsRejectsMisalignedMetadata(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	files := []types.LocationInfo{{Path: "/library/one.jpg", Hash: "hash-per-file-misaligned", Size: 11, ModTime: 21, Extension: ".jpg"}}
	if _, err := client.TagKnownFilesWithBackgroundTasksByFileTags(files, nil, nil, nil); err == nil {
		t.Fatal("expected misaligned per-file tags to fail")
	}
	exists, err := client.ContentExists(files[0].Hash)
	if err != nil {
		t.Fatalf("ContentExists: %v", err)
	}
	if exists {
		t.Fatal("content registration committed despite invalid per-file metadata")
	}
}

func sameStringSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := make(map[string]int, len(got))
	for _, value := range got {
		counts[value]++
	}
	for _, value := range want {
		counts[value]--
	}
	for _, count := range counts {
		if count != 0 {
			return false
		}
	}
	return true
}
