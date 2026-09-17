package database

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gooru.local/types"
)

func TestNewStoreSecuresSQLiteFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	store, err := NewStore(dbPath, false)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, err := store.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := store.Exec(`INSERT INTO t DEFAULT VALUES`); err != nil {
		t.Fatalf("insert row: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0600 {
			t.Fatalf("mode for %s = %o, want 600", path, got)
		}
	}
}

func newMemoryTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatalf("open memory database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	return &Store{DB: db, logger: log.New(io.Discard, "", 0)}
}

func TestLocationPublicIDsArePersistedAndResolved(t *testing.T) {
	store := newMemoryTestStore(t)
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES (?)`, "hash-one"); err != nil {
		t.Fatalf("insert content: %v", err)
	}
	if err := store.GetOrCreateLocation(store.DB, "hash-one", "/library/photo.jpg", 123, 456, ".jpg"); err != nil {
		t.Fatalf("GetOrCreateLocation: %v", err)
	}

	file, err := store.GetFileInfoByPath("/library/photo.jpg")
	if err != nil {
		t.Fatalf("GetFileInfoByPath: %v", err)
	}
	if file.PublicID == "" {
		t.Fatal("expected public id")
	}
	if strings.Contains(file.PublicID, "loc:") || strings.Contains(file.PublicID, "/library/photo.jpg") {
		t.Fatalf("public id exposed internal data: %q", file.PublicID)
	}
	resolved, err := store.GetLocationIDByPublicID(file.PublicID)
	if err != nil {
		t.Fatalf("GetLocationIDByPublicID: %v", err)
	}
	if resolved != file.ID {
		t.Fatalf("resolved id = %d, want %d", resolved, file.ID)
	}
	got, err := store.GetLocationPublicID(file.ID)
	if err != nil {
		t.Fatalf("GetLocationPublicID: %v", err)
	}
	if got != file.PublicID {
		t.Fatalf("public id lookup = %q, want %q", got, file.PublicID)
	}
}

func TestBatchUpsertLocationsPersistsExplicitAddedAtWithoutRewritingExistingValue(t *testing.T) {
	store := newMemoryTestStore(t)
	if err := store.BatchInsertContents(store.DB, []string{"hash-one", "hash-two"}); err != nil {
		t.Fatalf("BatchInsertContents: %v", err)
	}
	path := "/library/ordered.jpg"
	if err := store.BatchUpsertLocations(store.DB, map[string]types.LocationInfo{path: {Path: path, Hash: "hash-one", Size: 1, ModTime: 10, AddedAt: 1234, Extension: ".jpg"}}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	file, err := store.GetFileInfoByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.AddedAt != 1234*1000 {
		t.Fatalf("added_at=%d want %d", file.AddedAt, int64(1234*1000))
	}
	if err := store.BatchUpsertLocations(store.DB, map[string]types.LocationInfo{path: {Path: path, Hash: "hash-two", Size: 2, ModTime: 20, AddedAt: 9999, Extension: ".jpg"}}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	file, err = store.GetFileInfoByPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if file.AddedAt != 1234*1000 {
		t.Fatalf("existing added_at changed to %d, want stable %d", file.AddedAt, int64(1234*1000))
	}

	defaultPath := "/library/default-added.jpg"
	if err := store.BatchUpsertLocations(store.DB, map[string]types.LocationInfo{defaultPath: {Path: defaultPath, Hash: "hash-one", Size: 3, ModTime: 30, Extension: ".jpg"}}); err != nil {
		t.Fatalf("default added_at upsert: %v", err)
	}
	file, err = store.GetFileInfoByPath(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if file.AddedAt <= 0 {
		t.Fatalf("default added_at=%d, want insertion timestamp", file.AddedAt)
	}
}

func TestTagSuggestionsUseUniqueFileAggregatesForBaseAndNamespace(t *testing.T) {
	store := newMemoryTestStore(t)
	if _, err := store.Exec(`INSERT INTO contents (hash) VALUES ('h1'), ('h2'), ('h3')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Exec(`INSERT INTO tags (key, value) VALUES ('animal',''),('animal','cat'),('animal','hamster'),('animal','horse'),('ai',''),('a','')`); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][3]string{
		{"h1", "animal", ""}, {"h1", "animal", "cat"}, {"h1", "animal", "horse"},
		{"h2", "animal", "hamster"}, {"h3", "animal", "horse"},
		{"h1", "ai", ""}, {"h2", "ai", ""}, {"h3", "a", ""},
	} {
		if _, err := store.Exec(`INSERT INTO content_tags (content_hash, tag_id) SELECT ?, id FROM tags WHERE key = ? AND value = ?`, pair[0], pair[1], pair[2]); err != nil {
			t.Fatal(err)
		}
	}

	tags, err := store.ListTagSuggestions("a", 20)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, tag := range tags {
		counts[tag.Tag] = tag.Count
	}
	if counts["animal"] != 3 {
		t.Fatalf("animal count=%d want 3: %#v", counts["animal"], tags)
	}
	if counts["ai"] != 2 || counts["a"] != 1 {
		t.Fatalf("plain counts wrong: %#v", tags)
	}

	namespaces, err := store.ListNamespaceSuggestions("a", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(namespaces) != 1 || namespaces[0].Tag != "animal:" || namespaces[0].Count != 3 {
		t.Fatalf("namespace suggestions=%#v want animal: count 3", namespaces)
	}
}

type sizeToHashesRowsStub struct {
	sizes       []int64
	hashes      []string
	index       int
	terminalErr error
}

func (r *sizeToHashesRowsStub) Next() bool {
	return r.index < len(r.sizes)
}

func (r *sizeToHashesRowsStub) Scan(dest ...any) error {
	*(dest[0].(*int64)) = r.sizes[r.index]
	*(dest[1].(*string)) = r.hashes[r.index]
	r.index++
	return nil
}

func (r *sizeToHashesRowsStub) Err() error {
	return r.terminalErr
}

func TestSizeToHashesMapFromRowsPropagatesTerminalError(t *testing.T) {
	terminalErr := errors.New("terminal row iteration failure")
	rows := &sizeToHashesRowsStub{
		sizes:       []int64{123},
		hashes:      []string{"hash-one"},
		terminalErr: terminalErr,
	}

	got, err := sizeToHashesMapFromRows(rows)
	if !errors.Is(err, terminalErr) {
		t.Fatalf("error = %v, want terminal iterator error", err)
	}
	if got != nil {
		t.Fatalf("map = %#v, want nil on terminal iterator error", got)
	}
}
