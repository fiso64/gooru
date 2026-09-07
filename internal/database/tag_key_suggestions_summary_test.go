package database

import (
	"database/sql"
	"strings"
	"testing"
)

func TestTagKeySuggestionSummaryPrefixUsesRangeSearch(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`EXPLAIN QUERY PLAN
		SELECT key AS tag_str, files_count
		FROM tag_key_counts
		WHERE files_count > 0 AND key LIKE ?
		ORDER BY files_count DESC, tag_str ASC
		LIMIT ?`, "art%", 20)
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
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.ToLower(strings.Join(details, "\n"))
	if strings.Contains(plan, "scan tag_key_counts") || !strings.Contains(plan, "search tag_key_counts") {
		t.Fatalf("tag key suggestion summary does not use a prefix range search:\n%s", plan)
	}
}
