package gooru

import (
	"errors"
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

func TestTagKnownFilesByHashTagsRollsBackExistingAndNewContentTogether(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	existing := types.LocationInfo{Path: "/library/existing.jpg", Hash: "hash-atomic-existing", Size: 11, ModTime: 21, Extension: ".jpg"}
	if _, err := client.TagKnownFilesWithBackgroundTasksByFileTags([]types.LocationInfo{existing}, [][]string{{"seed"}}, nil, nil); err != nil {
		t.Fatalf("seed existing content: %v", err)
	}
	fresh := types.LocationInfo{Path: "/library/fresh.jpg", Hash: "hash-atomic-fresh", Size: 12, ModTime: 22, Extension: ".jpg"}
	_, err := client.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationState(
		[]types.LocationInfo{fresh},
		map[string][]string{existing.Hash: {"duplicate:tag"}, fresh.Hash: {"fresh:tag"}},
		nil,
		func(int) (BackgroundOperationTransactionState, error) {
			return BackgroundOperationTransactionState{}, errors.New("stop before commit")
		},
		nil,
	)
	if err == nil {
		t.Fatal("expected state builder failure")
	}
	tags, err := client.store.GetTagsForContent(existing.Hash)
	if err != nil {
		t.Fatalf("GetTagsForContent(existing): %v", err)
	}
	if !sameStringSet(tags, []string{"seed"}) {
		t.Fatalf("existing tags committed despite rollback: %#v", tags)
	}
	exists, err := client.ContentExists(fresh.Hash)
	if err != nil {
		t.Fatalf("ContentExists(fresh): %v", err)
	}
	if exists {
		t.Fatal("fresh content committed despite rollback")
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
