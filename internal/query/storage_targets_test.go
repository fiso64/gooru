package query

import (
	"reflect"
	"strings"
	"testing"
)

func TestStorageTargetMetaTagsParseAndBuild(t *testing.T) {
	for _, raw := range []string{"@external", "@in_target:any", "@in_target:archive"} {
		expr, err := Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if err := ValidateAST(expr); err != nil {
			t.Fatalf("validate %q: %v", raw, err)
		}
	}
	if _, err := ParseMetaTag("@in_target:"); err == nil {
		t.Fatal("expected empty @in_target value to fail")
	}
	if _, err := ParseMetaTag("@external:any"); err == nil {
		t.Fatal("expected @external value to fail")
	}

	expr, err := Parse("@in_target:archive")
	if err != nil {
		t.Fatal(err)
	}
	contentSQL, contentArgs := Build(expr, nil)
	if !strings.Contains(contentSQL, "SELECT DISTINCT l.content_hash as hash FROM managed_storage_target_locations mstl JOIN locations l ON l.id = mstl.location_id WHERE mstl.target_id = ?") {
		t.Fatalf("unexpected content target SQL: %s", contentSQL)
	}
	if !reflect.DeepEqual(contentArgs, []interface{}{"archive"}) {
		t.Fatalf("content target args=%#v", contentArgs)
	}
	locationSQL, locationArgs := BuildLocations(expr, nil)
	if !strings.Contains(locationSQL, "SELECT l.id as id FROM managed_storage_target_locations mstl JOIN locations l ON l.id = mstl.location_id WHERE mstl.target_id = ?") {
		t.Fatalf("unexpected location target SQL: %s", locationSQL)
	}
	if !reflect.DeepEqual(locationArgs, []interface{}{"archive"}) {
		t.Fatalf("location target args=%#v", locationArgs)
	}
}

func TestStorageTargetAnyAndExternalUseComplementaryMembershipPredicates(t *testing.T) {
	anyExpr, err := Parse("@in_target:any")
	if err != nil {
		t.Fatal(err)
	}
	anyContentSQL, anyContentArgs := Build(anyExpr, nil)
	if len(anyContentArgs) != 0 || strings.Contains(anyContentSQL, "target_id = ?") {
		t.Fatalf("@in_target:any unexpectedly filters a target: %s args=%#v", anyContentSQL, anyContentArgs)
	}
	if !strings.Contains(anyContentSQL, "managed_storage_target_locations") {
		t.Fatalf("@in_target:any does not use target membership: %s", anyContentSQL)
	}

	externalExpr, err := Parse("@external")
	if err != nil {
		t.Fatal(err)
	}
	externalContentSQL, args := Build(externalExpr, nil)
	if len(args) != 0 {
		t.Fatalf("@external args=%#v", args)
	}
	for _, fragment := range []string{
		"NOT EXISTS",
		"FROM locations ml JOIN managed_storage_target_locations mstl ON mstl.location_id = ml.id",
		"ml.content_hash = l.content_hash",
	} {
		if !strings.Contains(externalContentSQL, fragment) {
			t.Fatalf("content @external SQL missing %q: %s", fragment, externalContentSQL)
		}
	}

	externalLocationSQL, locationArgs := BuildLocations(externalExpr, nil)
	if len(locationArgs) != 0 {
		t.Fatalf("location @external args=%#v", locationArgs)
	}
	if !strings.Contains(externalLocationSQL, "NOT EXISTS (SELECT 1 FROM managed_storage_target_locations mstl WHERE mstl.location_id = l.id)") {
		t.Fatalf("unexpected location @external SQL: %s", externalLocationSQL)
	}
}
