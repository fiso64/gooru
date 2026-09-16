package database

import (
	"database/sql"
	"strings"
	"testing"
)

func explainPlan(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("explain query: %v", err)
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
	return strings.Join(details, "\n")
}

func migratedPerformanceDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := RunMigrations(db); err != nil {
		db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	return db
}

func TestDefaultLibrarySortUsesExactDirectionIndex(t *testing.T) {
	db := migratedPerformanceDB(t)
	defer db.Close()

	plan := explainPlan(t, db, `
		SELECT l.id, l.path
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		ORDER BY l.added_at DESC, l.id ASC
		LIMIT 61 OFFSET 0
	`)
	if !strings.Contains(plan, "idx_locations_added_at_desc_id_asc") {
		t.Fatalf("default library query did not use mixed-direction index:\n%s", plan)
	}
	if strings.Contains(plan, "USE TEMP B-TREE FOR ORDER BY") {
		t.Fatalf("default library query still spills into a temporary ORDER BY sort:\n%s", plan)
	}
}

func TestExtensionFilterUsesCaseInsensitiveExpressionIndex(t *testing.T) {
	db := migratedPerformanceDB(t)
	defer db.Close()

	plan := explainPlan(t, db, `
		SELECT id
		FROM locations
		WHERE lower(extension) = lower(?)
	`, ".CBZ")
	if !strings.Contains(plan, "idx_locations_extension_lower") {
		t.Fatalf("case-insensitive extension filter did not use expression index:\n%s", plan)
	}
	if strings.Contains(plan, "SCAN locations") {
		t.Fatalf("case-insensitive extension filter still scans locations:\n%s", plan)
	}
}

func TestExtensionFilterDefaultSortUsesCoveringOrderIndex(t *testing.T) {
	db := migratedPerformanceDB(t)
	defer db.Close()

	plan := explainPlan(t, db, `
		SELECT l.id, l.path
		FROM locations l
		LEFT JOIN media_metadata mm ON mm.content_hash = l.content_hash
		WHERE lower(l.extension) = lower(?)
		ORDER BY l.added_at DESC, l.id ASC
		LIMIT 61 OFFSET 0
	`, ".CBZ")
	if !strings.Contains(plan, "idx_locations_extension_lower_added_at_desc_id_asc") {
		t.Fatalf("extension-filtered default library query did not use composite index:\n%s", plan)
	}
	if strings.Contains(plan, "USE TEMP B-TREE FOR ORDER BY") {
		t.Fatalf("extension-filtered default library query still spills into a temporary ORDER BY sort:\n%s", plan)
	}
}

func TestBackgroundOperationAdmissionUsesPendingKindIndex(t *testing.T) {
	db := migratedPerformanceDB(t)
	defer db.Close()

	plan := explainPlan(t, db, `
		SELECT COUNT(*)
		FROM background_operations
		WHERE kind = ? AND status = 'pending'
	`, "upload")
	if !strings.Contains(plan, "background_operations_pending_kind_idx") {
		t.Fatalf("pending background operation admission count did not use kind index:\n%s", plan)
	}
	if strings.Contains(plan, "SCAN background_operations") {
		t.Fatalf("pending background operation admission count still scans operation history:\n%s", plan)
	}
}

func TestActiveBackgroundOperationLookupUsesKindAndRecencyIndex(t *testing.T) {
	db := migratedPerformanceDB(t)
	defer db.Close()

	plan := explainPlan(t, db, `
		SELECT id
		FROM background_operations
		WHERE kind = ? AND status IN ('pending', 'running')
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, "media-metadata-sweep")
	if !strings.Contains(plan, "background_operations_active_kind_created_idx") {
		t.Fatalf("active background operation lookup did not use kind/recency index:\n%s", plan)
	}
	if strings.Contains(plan, "SCAN background_operations") {
		t.Fatalf("active background operation lookup still scans operation history:\n%s", plan)
	}
	if strings.Contains(plan, "USE TEMP B-TREE FOR ORDER BY") {
		t.Fatalf("active background operation lookup still sorts operation history:\n%s", plan)
	}
}
