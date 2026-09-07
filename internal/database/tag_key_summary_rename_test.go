package database

import (
	"database/sql"
	"reflect"
	"testing"
)

func TestTagKeySummariesTrackCrossNamespaceRename(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	for _, hash := range []string{"a", "b"} {
		if _, err := db.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
		insertTagKindTestLocation(t, db, hash, hash+"-jpg", "/"+hash+".jpg", ".jpg")
	}
	alice := insertTagKeyKindTestTag(t, db, "artist", "alice")
	bob := insertTagKeyKindTestTag(t, db, "artist", "bob")
	existingCreator := insertTagKeyKindTestTag(t, db, "creator", "existing")
	for _, pair := range []struct {
		hash string
		id   int64
	}{{"a", alice}, {"a", bob}, {"b", alice}, {"a", existingCreator}} {
		if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES (?, ?)`, pair.hash, pair.id); err != nil {
			t.Fatal(err)
		}
	}

	assertAllKeySummariesMatchRecomputed(t, db)

	// Store.RenameTag updates the tag row in place. After crossing namespaces,
	// a remains in artist via bob and was already in creator, while b moves from
	// artist to creator. Every key-indexed summary must preserve that dedup.
	if _, err := db.Exec(`UPDATE tags SET key = 'creator', value = 'alice' WHERE id = ?`, alice); err != nil {
		t.Fatal(err)
	}
	assertAllKeySummariesMatchRecomputed(t, db)

	// Value-only changes affect namespace membership even though the key stays
	// the same; the rename trigger deliberately rebuilds that key as well.
	if _, err := db.Exec(`UPDATE tags SET value = '' WHERE id = ?`, bob); err != nil {
		t.Fatal(err)
	}
	assertAllKeySummariesMatchRecomputed(t, db)
}

func assertAllKeySummariesMatchRecomputed(t *testing.T, db *sql.DB) {
	t.Helper()
	assertTagKeyCountsMatchRecomputed(t, db)

	wantNamespace := readStringCounts(t, db, `
		SELECT t.key, COUNT(DISTINCT ct.content_hash)
		FROM tags t JOIN content_tags ct ON ct.tag_id = t.id
		WHERE t.value != ''
		GROUP BY t.key
	`)
	gotNamespace := readStringCounts(t, db, `SELECT key, files_count FROM tag_namespace_counts WHERE files_count > 0`)
	if !reflect.DeepEqual(gotNamespace, wantNamespace) {
		t.Fatalf("tag_namespace_counts = %#v, recomputed = %#v", gotNamespace, wantNamespace)
	}

	rows, err := db.Query(`SELECT DISTINCT key FROM tags ORDER BY key`)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, key := range keys {
		assertTagKeyKindCountsMatchRecomputed(t, db, key)
	}
}
