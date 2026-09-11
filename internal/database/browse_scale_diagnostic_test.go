package database

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"testing"
	"time"
)

func TestDiagnosticBrowseScale548(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	store := &Store{DB: db, logger: log.New(io.Discard, "", 0)}

	tx, err := db.Begin()
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
	defer contentStmt.Close()
	locationStmt, err := tx.Prepare(`INSERT INTO locations (public_id, content_hash, path, size_bytes, mod_time, added_at, extension) VALUES (?, ?, ?, 1, 1, ?, '.jpg')`)
	if err != nil {
		t.Fatal(err)
	}
	defer locationStmt.Close()
	assocStmt, err := tx.Prepare(`INSERT INTO content_tags (content_hash, tag_id) VALUES (?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer assocStmt.Close()
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
		const iterations = 100
		for i := 0; i < iterations; i++ {
			if err := fn(); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("#548 %s: %s/op", name, time.Since(start)/iterations)
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