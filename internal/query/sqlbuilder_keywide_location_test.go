package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildBareTagLocationQueryUsesOrderedLocationProbeShape(t *testing.T) {
	expr, err := Parse("hidden")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateAST(expr); err != nil {
		t.Fatal(err)
	}

	locationQuery, args := BuildLocations(expr, nil)
	if strings.Contains(locationQuery, "DISTINCT") {
		t.Fatalf("bare key location query should not materialize a DISTINCT location set: %s", locationQuery)
	}
	if !strings.Contains(locationQuery, "FROM locations l WHERE EXISTS") {
		t.Fatalf("bare key location query should preserve location-driven ordering: %s", locationQuery)
	}
	if !strings.Contains(locationQuery, "ct.content_hash = l.content_hash") || !strings.Contains(locationQuery, "t.key = ?") {
		t.Fatalf("bare key location query should probe indexed tag associations: %s", locationQuery)
	}
	if !reflect.DeepEqual(args, []interface{}{"hidden"}) {
		t.Fatalf("location args = %#v", args)
	}

	contentQuery, _ := Build(expr, nil)
	if !strings.Contains(contentQuery, "SELECT DISTINCT") {
		t.Fatalf("content-oriented bare key queries should keep existing semantics: %s", contentQuery)
	}
}

func TestBuildExplicitEmptyTagDoesNotUseBareKeyProbe(t *testing.T) {
	expr, err := Parse("hidden:")
	if err != nil {
		t.Fatal(err)
	}
	query, _ := BuildLocations(expr, nil)
	if strings.Contains(query, "WHERE EXISTS") {
		t.Fatalf("explicit empty-value tag must retain exact empty-value semantics: %s", query)
	}
	if !strings.Contains(query, "t.value = ''") {
		t.Fatalf("explicit empty-value tag lost value predicate: %s", query)
	}
}
