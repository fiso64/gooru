package database

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestDiagnosticEncryptedBrowseScale548(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	key := make([]byte, encryptedDatabaseKeySize)
	for i := range key {
		key[i] = byte(i + 1)
	}
	store, err := NewEncryptedStore(path, false, key)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := RunMigrations(store.DB); err != nil {
		t.Fatal(err)
	}

	tx, err := store.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO tags (key, value) VALUES ('remove', '')`); err != nil {
		t.Fatal(err)
	}
	var tagID int64
	if err := tx.QueryRow(`SELECT id FROM tags WHERE key = 'remove' AND value = ''`).Scan(&tagID); err != nil {
		t.Fatal(err)
	}
	contentStmt, err := tx.Prepare(`INSERT INTO contents (hash) VALUES (?)`)
	if err != nil {
		t.Fatal(err)
	}
	locationStmt, err := tx.Prepare(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, added_at, extension) VALUES (?, ?, ?, 1, 1, ?, '.jpg')`)
	if err != nil {
		t.Fatal(err)
	}
	assocStmt, err := tx.Prepare(`INSERT INTO content_tags (content_hash, tag_id) VALUES (?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		hash := fmt.Sprintf("hash-%05d", i)
		if _, err := contentStmt.Exec(hash); err != nil {
			t.Fatal(err)
		}
		if _, err := locationStmt.Exec(fmt.Sprintf("file_%05d", i), hash, fmt.Sprintf("/library/%05d.jpg", i), i+1); err != nil {
			t.Fatal(err)
		}
		if _, err := assocStmt.Exec(hash, tagID); err != nil {
			t.Fatal(err)
		}
	}
	_ = contentStmt.Close()
	_ = locationStmt.Close()
	_ = assocStmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	locationQuery := `SELECT l.id as id FROM locations l WHERE EXISTS (SELECT 1 FROM content_tags ct JOIN tags t ON ct.tag_id = t.id WHERE ct.content_hash = l.content_hash AND t.key = ?)`
	measure := func(name string, fn func() error) {
		t.Helper()
		for i := 0; i < 5; i++ {
			if err := fn(); err != nil {
				t.Fatal(err)
			}
		}
		start := time.Now()
		const iterations = 50
		for i := 0; i < iterations; i++ {
			if err := fn(); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("#548 encrypted %s: %s/op", name, time.Since(start)/iterations)
	}

	measure("unfiltered-page", func() error {
		_, err := store.GetAllFilesInfoPageSorted(61, nil, "added", "desc")
		return err
	})
	measure("root-facets", func() error {
		_, err := store.KindFacets()
		return err
	})
	measure("root-count", func() error {
		_, err := store.CountAllFiles()
		return err
	})
	measure("tagged-page", func() error {
		_, err := store.GetFilesInfoByLocationQueryPageSorted(locationQuery, []interface{}{"remove"}, 61, nil, "added", "desc")
		return err
	})
	measure("tagged-facets", func() error {
		_, err := store.KindFacetsForTagKey("remove")
		return err
	})
}