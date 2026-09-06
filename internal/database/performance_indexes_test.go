package database

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLibrarySortUsesExactDirectionIndex(t *testing.T) {
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "gooru.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	rows, err := db.Query(`
		EXPLAIN QUERY PLAN
		SELECT l.id, l.path
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.location_id = l.id
		ORDER BY l.added_at DESC, l.id ASC
		LIMIT 61 OFFSET 0
	`)
	if err != nil {
		t.Fatalf("explain default library query: %v", err)
	}
	defer rows.Close()

	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan query plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read query plan: %v", err)
	}

	plan := strings.Join(details, "\n")
	if !strings.Contains(plan, "idx_locations_added_at_desc_id_asc") {
		t.Fatalf("default library query did not use mixed-direction index:\n%s", plan)
	}
	if strings.Contains(plan, "USE TEMP B-TREE FOR ORDER BY") {
		t.Fatalf("default library query still spills into a temporary ORDER BY sort:\n%s", plan)
	}
}
