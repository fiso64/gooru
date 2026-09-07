package database

import (
	"database/sql"
	"strings"
	"testing"
)

func TestBoundedTagCountsUseRankIndexesWithoutGlobalSort(t *testing.T) {
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
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.ToLower(strings.Join(details, "\n"))
	if strings.Contains(plan, "use temp b-tree for order by") {
		t.Fatalf("bounded tag-count ranking still globally sorts summaries:\n%s", plan)
	}
	for _, index := range []string{"idx_tag_key_counts_rank", "idx_tags_value_rank"} {
		if !strings.Contains(plan, strings.ToLower(index)) {
			t.Fatalf("bounded tag-count ranking does not use %s:\n%s", index, plan)
		}
	}
}
