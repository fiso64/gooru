package query

import (
	"strings"
	"testing"
)

func TestBuildLocationsUsesAssociationPlanForKnownEmptyBareKey(t *testing.T) {
	expr, err := Parse(`-"hidden"`)
	if err != nil {
		t.Fatal(err)
	}
	sqlQuery, args := BuildLocations(expr, map[string]int{"hidden": 0})
	associationPlan := `SELECT DISTINCT l.id as id FROM tags t JOIN content_tags ct ON ct.tag_id = t.id JOIN locations l ON l.content_hash = ct.content_hash WHERE t.key = ?`
	if !strings.Contains(sqlQuery, associationPlan) {
		t.Fatalf("known-empty bare key should use association-driven plan, got %s", sqlQuery)
	}
	if len(args) != 1 || args[0] != "hidden" {
		t.Fatalf("args=%v want [hidden]", args)
	}
}

func TestBuildLocationsKeepsLocationPlanForPopulatedBareKey(t *testing.T) {
	expr, err := Parse(`-"hidden"`)
	if err != nil {
		t.Fatal(err)
	}
	sqlQuery, args := BuildLocations(expr, map[string]int{"hidden": 10})
	if !strings.Contains(sqlQuery, `SELECT l.id as id FROM locations l WHERE EXISTS (`) {
		t.Fatalf("populated bare key should keep location-driven plan, got %s", sqlQuery)
	}
	if len(args) != 1 || args[0] != "hidden" {
		t.Fatalf("args=%v want [hidden]", args)
	}
}
