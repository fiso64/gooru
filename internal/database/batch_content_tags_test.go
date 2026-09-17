package database

import (
	"fmt"
	"reflect"
	"testing"
)

func TestBatchGetTagsForContentsChunksAndDeduplicatesHashes(t *testing.T) {
	store := newMemoryTestStore(t)

	hashes := make([]string, maxVars+1)
	for i := range hashes {
		hashes[i] = fmt.Sprintf("hash-%04d", i)
	}
	if err := store.BatchInsertContents(store.DB, hashes); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}
	if _, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('z',''), ('a',''), ('ns','value')`); err != nil {
		t.Fatalf("insert tags: %v", err)
	}
	for _, pair := range [][3]string{
		{hashes[0], "z", ""},
		{hashes[0], "a", ""},
		{hashes[maxVars], "ns", "value"},
	} {
		if _, err := store.Exec(`
			INSERT INTO content_tags (content_hash, tag_id)
			SELECT ?, id FROM tags WHERE key = ? AND value = ?`, pair[0], pair[1], pair[2]); err != nil {
			t.Fatalf("associate tag: %v", err)
		}
	}

	requested := append(append([]string(nil), hashes...), hashes[0])
	got, err := store.BatchGetTagsForContents(requested)
	if err != nil {
		t.Fatalf("BatchGetTagsForContents: %v", err)
	}

	want := map[string][]string{
		hashes[0]:       {"a", "z"},
		hashes[maxVars]: {"ns:value"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tags = %#v, want %#v", got, want)
	}
}

func TestBatchGetTagsForContentsEmptyInput(t *testing.T) {
	store := newMemoryTestStore(t)
	got, err := store.BatchGetTagsForContents(nil)
	if err != nil {
		t.Fatalf("BatchGetTagsForContents: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("tags = %#v, want empty map", got)
	}
}
