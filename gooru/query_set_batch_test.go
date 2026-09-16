package gooru

import (
	"fmt"
	"testing"
)

func replacementTagsForBatchTest(count int) []string {
	tags := make([]string, count)
	for i := range tags {
		tags[i] = fmt.Sprintf("replacement:%04d", i)
	}
	return tags
}

func assertReplacementTagsForBatchTest(t *testing.T, client *Client, path string, replacements []string) {
	t.Helper()
	tags, _, err := client.GetTagsForFile(path, false)
	if err != nil {
		t.Fatalf("read replacement tags: %v", err)
	}
	if len(tags) != len(replacements) {
		t.Fatalf("replacement tag count = %d, want %d", len(tags), len(replacements))
	}
	if containsBackgroundTag(tags, "group:one") || containsBackgroundTag(tags, "old") {
		t.Fatalf("set mutation retained old tags: %v", tags)
	}
	if !containsBackgroundTag(tags, replacements[0]) || !containsBackgroundTag(tags, replacements[len(replacements)-1]) {
		t.Fatalf("set mutation missed replacement batch boundaries: first=%q last=%q", replacements[0], replacements[len(replacements)-1])
	}
}

func TestBackgroundSetTagMutationBatchesReplacementTags(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "body")
	if _, err := client.TagFiles([]string{path}, []string{"group:one", "old"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	replacements := replacementTagsForBatchTest(1000)
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "set",
		Selector:   map[string]string{"query": "group:one"},
		Tags:       replacements,
		Query:      "group:one",
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("execute mutation: %v", err)
	}

	assertReplacementTagsForBatchTest(t, client, path, replacements)
}

func TestSetTagsForFilesByQueryExcludingBatchesReplacementTags(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "body")
	if _, err := client.TagFiles([]string{path}, []string{"group:one", "old"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	replacements := replacementTagsForBatchTest(1000)
	affected, err := client.SetTagsForFilesByQueryExcluding("group:one", replacements, nil)
	if err != nil {
		t.Fatalf("set query tags: %v", err)
	}
	if affected != 1 {
		t.Fatalf("affected files = %d, want 1", affected)
	}

	assertReplacementTagsForBatchTest(t, client, path, replacements)
}
