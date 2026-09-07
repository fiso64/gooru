package database

import (
	"strings"
	"testing"
)

func TestExactTagFilterUsesCoveringAssociationIndex(t *testing.T) {
	db := migratedPerformanceDB(t)
	defer db.Close()

	plan := explainPlan(t, db, `
		SELECT DISTINCT l.id
		FROM locations l
		JOIN content_tags ct ON l.content_hash = ct.content_hash
		JOIN tags t ON ct.tag_id = t.id
		WHERE t.key = ? AND t.value = ?
	`, "artist", "alice")
	if !strings.Contains(plan, "idx_content_tags_tag_id_content_hash") {
		t.Fatalf("exact tag filter did not use covering association index:\n%s", plan)
	}
	if !strings.Contains(plan, "COVERING INDEX idx_content_tags_tag_id_content_hash") {
		t.Fatalf("exact tag filter still needs content_tags table lookups:\n%s", plan)
	}
}
