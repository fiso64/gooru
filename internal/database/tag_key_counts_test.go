package database

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestTagKeyCountsMigrationBackfillsAndTracksDistinctContents(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	migrations, err := loadEmbeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureMigrationTable(db); err != nil {
		t.Fatal(err)
	}
	for _, migration := range migrations {
		if migration.version >= 13 {
			break
		}
		if err := applyMigration(db, migration); err != nil {
			t.Fatalf("apply migration %d: %v", migration.version, err)
		}
	}

	insertContent := func(hash string) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO contents (hash) VALUES (?)`, hash); err != nil {
			t.Fatal(err)
		}
	}
	insertTag := func(key, value string) int64 {
		t.Helper()
		res, err := db.Exec(`INSERT INTO tags (key, value) VALUES (?, ?)`, key, value)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	associate := func(hash string, tagID int64) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO content_tags (content_hash, tag_id) VALUES (?, ?)`, hash, tagID); err != nil {
			t.Fatal(err)
		}
	}

	insertContent("a")
	insertContent("b")
	insertContent("c")
	aliceID := insertTag("artist", "alice")
	bobID := insertTag("artist", "bob")
	favoriteID := insertTag("favorite", "")
	associate("a", aliceID)
	associate("a", bobID)
	associate("b", aliceID)
	associate("a", favoriteID)

	if err := RunMigrations(db); err != nil {
		t.Fatalf("RunMigrations through v13: %v", err)
	}
	assertTagKeyCountsMatchRecomputed(t, db)
	assertTagValueCountsMatchRecomputed(t, db)

	carolID := insertTag("artist", "carol")
	associate("a", carolID) // already has artist tags: distinct key count must not change.
	assertTagKeyCountsMatchRecomputed(t, db)

	associate("c", carolID) // first artist tag for c: distinct key count increments.
	assertTagKeyCountsMatchRecomputed(t, db)
	assertTagValueCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`DELETE FROM content_tags WHERE content_hash = 'a' AND tag_id = ?`, aliceID); err != nil {
		t.Fatal(err)
	}
	assertTagKeyCountsMatchRecomputed(t, db) // a still has bob/carol.

	if _, err := db.Exec(`DELETE FROM content_tags WHERE content_hash = 'a' AND tag_id = ?`, bobID); err != nil {
		t.Fatal(err)
	}
	assertTagKeyCountsMatchRecomputed(t, db) // a still has carol.

	if _, err := db.Exec(`DELETE FROM content_tags WHERE content_hash = 'a' AND tag_id = ?`, carolID); err != nil {
		t.Fatal(err)
	}
	assertTagKeyCountsMatchRecomputed(t, db) // last artist association for a decrements exactly once.
	assertTagValueCountsMatchRecomputed(t, db)

	if _, err := db.Exec(`DELETE FROM content_tags WHERE content_hash = 'a'`); err != nil {
		t.Fatal(err)
	}
	assertTagKeyCountsMatchRecomputed(t, db)
	assertTagValueCountsMatchRecomputed(t, db)

	// Direct tag deletion is not the normal cleanup path, but FK cascades must
	// still route through count maintenance while the tag key is queryable.
	if _, err := db.Exec(`DELETE FROM tags WHERE id = ?`, carolID); err != nil {
		t.Fatal(err)
	}
	assertTagKeyCountsMatchRecomputed(t, db)
	assertTagValueCountsMatchRecomputed(t, db)
}

func TestTagCountSummaryPlanDoesNotReadContentAssociations(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}

	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT key AS tag_str, files_count AS final_count
		FROM tag_key_counts
		WHERE files_count > 0
		UNION ALL
		SELECT key || ':' || value AS tag_str, files_count AS final_count
		FROM tags
		WHERE value != '' AND files_count > 0
		ORDER BY final_count DESC, tag_str ASC
		LIMIT 200`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		details = append(details, detail)
	}
	plan := strings.ToLower(strings.Join(details, "\n"))
	if strings.Contains(plan, "content_tags") {
		t.Fatalf("tag counts query still reads content_tags:\n%s", plan)
	}
	if !strings.Contains(plan, "tag_key_counts") || !strings.Contains(plan, "tags") {
		t.Fatalf("tag counts query does not use maintained summary tables:\n%s", plan)
	}
}

func assertTagKeyCountsMatchRecomputed(t *testing.T, db *sql.DB) {
	t.Helper()
	want := readStringCounts(t, db, `
		SELECT t.key, COUNT(DISTINCT ct.content_hash)
		FROM tags t
		JOIN content_tags ct ON ct.tag_id = t.id
		GROUP BY t.key
	`)
	got := readStringCounts(t, db, `SELECT key, files_count FROM tag_key_counts WHERE files_count > 0`)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tag_key_counts = %#v, recomputed = %#v", got, want)
	}
}

func assertTagValueCountsMatchRecomputed(t *testing.T, db *sql.DB) {
	t.Helper()
	want := readStringCounts(t, db, `
		SELECT CAST(t.id AS TEXT), COUNT(ct.content_hash)
		FROM tags t
		LEFT JOIN content_tags ct ON ct.tag_id = t.id
		GROUP BY t.id
		HAVING COUNT(ct.content_hash) > 0
	`)
	got := readStringCounts(t, db, `SELECT CAST(id AS TEXT), files_count FROM tags WHERE files_count > 0`)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tag files_count = %#v, recomputed = %#v", got, want)
	}
}

func readStringCounts(t *testing.T, db *sql.DB, query string) map[string]int {
	t.Helper()
	rows, err := db.Query(query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			t.Fatal(err)
		}
		out[key] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
