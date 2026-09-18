package gooru

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestSavedSearchForUsernamePersistsAndExecutesQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	if _, err := client.store.Exec(`INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)`, "usr_test", "Alice", "unused-test-hash", "admin"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	catPath := filepath.Join(dir, "cat.jpg")
	dogPath := filepath.Join(dir, "dog.jpg")
	if err := os.WriteFile(catPath, []byte("cat bytes"), 0600); err != nil {
		t.Fatalf("write cat: %v", err)
	}
	if err := os.WriteFile(dogPath, []byte("dog bytes"), 0600); err != nil {
		t.Fatalf("write dog: %v", err)
	}
	if _, err := client.TagFiles([]string{catPath}, []string{"animal:cat"}, nil, false); err != nil {
		t.Fatalf("tag cat: %v", err)
	}
	if _, err := client.TagFiles([]string{dogPath}, []string{"animal:dog"}, nil, false); err != nil {
		t.Fatalf("tag dog: %v", err)
	}

	created, err := client.CreateSavedSearchForUsername("alice", "Cats", "animal:cat", "name", "asc")
	if err != nil {
		t.Fatalf("create saved search: %v", err)
	}
	if created.UserID != "usr_test" || created.Query != "animal:cat" || created.Sort != "name" || created.Order != "asc" {
		t.Fatalf("unexpected saved search: %+v", created)
	}

	files, err := client.SearchSavedSearchForUsername("ALICE", "cats", false)
	if err != nil {
		t.Fatalf("execute saved search: %v", err)
	}
	if len(files) != 1 || files[0].Path != catPath {
		t.Fatalf("expected only %q, got %+v", catPath, files)
	}

	if _, err := client.CreateSavedSearchForUsername("alice", "All", "", "name", "desc"); err != nil {
		t.Fatalf("create all-files saved search: %v", err)
	}
	files, err = client.SearchSavedSearchForUsername("alice", "all", false)
	if err != nil {
		t.Fatalf("execute all-files saved search: %v", err)
	}
	if len(files) != 2 || files[0].Path != dogPath || files[1].Path != catPath {
		t.Fatalf("expected name-desc order [%q %q], got %+v", dogPath, catPath, files)
	}
}

func TestCreateSavedSearchNormalizesSortAndRejectsInvalidQuery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()
	if _, err := client.store.Exec(`INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)`, "usr_test", "alice", "unused-test-hash", "admin"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	item, err := client.CreateSavedSearchForUsername("alice", "Everything", "", "nonsense", "nonsense")
	if err != nil {
		t.Fatalf("create normalized search: %v", err)
	}
	if item.Sort != "name" || item.Order != "asc" {
		t.Fatalf("expected normalized name/asc, got %s/%s", item.Sort, item.Order)
	}
	if _, err := client.CreateSavedSearchForUsername("alice", "Broken", "(", "name", "asc"); err == nil {
		t.Fatal("expected invalid query to be rejected")
	}
}

func TestSavedSearchReorderPersistsWithoutRewritingMetadata(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := Init(dbPath, types.StrategyPartial, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()
	if _, err := client.store.Exec(`INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)`, "usr_test", "alice", "unused-test-hash", "admin"); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	first, err := client.CreateSavedSearchForUsername("alice", "First", "kind:image", "name", "asc")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := client.CreateSavedSearchForUsername("alice", "Second", "kind:video", "size", "desc")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	third, err := client.CreateSavedSearchForUsername("alice", "Third", "", "name", "asc")
	if err != nil {
		t.Fatalf("create third: %v", err)
	}

	want := []string{third.ID, first.ID, second.ID}
	if err := client.ReorderSavedSearchesForUsername("alice", want); err != nil {
		t.Fatalf("reorder: %v", err)
	}
	items, err := client.ListSavedSearchesForUsername("alice")
	if err != nil {
		t.Fatalf("list reordered: %v", err)
	}
	for i, id := range want {
		if items[i].ID != id {
			t.Fatalf("order[%d] = %s, want %s", i, items[i].ID, id)
		}
	}
	if items[0].Name != "Third" || items[1].Name != "First" || items[1].Query != "kind:image" || items[2].Order != "desc" {
		t.Fatalf("reorder rewrote metadata: %+v", items)
	}

	first.Name = "First renamed"
	if _, err := client.UpdateSavedSearch(first); err != nil {
		t.Fatalf("update first metadata: %v", err)
	}
	items, err = client.ListSavedSearchesForUsername("alice")
	if err != nil {
		t.Fatalf("list after metadata update: %v", err)
	}
	for i, id := range want {
		if items[i].ID != id {
			t.Fatalf("metadata update changed order: got %+v, want %v", items, want)
		}
	}

	fourth, err := client.CreateSavedSearchForUsername("alice", "Fourth", "", "name", "asc")
	if err != nil {
		t.Fatalf("create fourth: %v", err)
	}
	items, err = client.ListSavedSearchesForUsername("alice")
	if err != nil {
		t.Fatalf("list after append: %v", err)
	}
	if items[len(items)-1].ID != fourth.ID {
		t.Fatalf("new search did not append: got last %s, want %s", items[len(items)-1].ID, fourth.ID)
	}

	if err := client.ReorderSavedSearches("usr_test", []string{first.ID, second.ID}); !errors.Is(err, ErrInvalidSavedSearchOrder) {
		t.Fatalf("incomplete reorder error = %v, want ErrInvalidSavedSearchOrder", err)
	}
	if err := client.ReorderSavedSearches("usr_test", []string{first.ID, second.ID, third.ID, third.ID}); !errors.Is(err, ErrInvalidSavedSearchOrder) {
		t.Fatalf("duplicate reorder error = %v, want ErrInvalidSavedSearchOrder", err)
	}
}
