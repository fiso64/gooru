package gooru

import (
	"reflect"
	"testing"
)

func TestExcludeContentHashes(t *testing.T) {
	query, args := excludeContentHashes("SELECT hash FROM locations WHERE extension = ?", []interface{}{`.jpg`}, []string{"hash-a", "hash-b", "hash-a", ""})
	wantQuery := "SELECT hash FROM (SELECT hash FROM locations WHERE extension = ?) WHERE hash NOT IN (?,?)"
	if query != wantQuery {
		t.Fatalf("query = %q, want %q", query, wantQuery)
	}
	wantArgs := []interface{}{`.jpg`, "hash-a", "hash-b"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}
