package query

import (
	"errors"
	"strings"
	"testing"
)

func TestExpandSavedSearchesPreservesGrouping(t *testing.T) {
	ast, err := Parse("@saved:search1 tag3")
	if err != nil {
		t.Fatal(err)
	}
	err = ExpandSavedSearches(ast, func(name string) (string, error) {
		if name != "search1" {
			return "", errors.New("not found")
		}
		return "tag1 | tag2", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := Format(ast)
	if got != `("tag1" | "tag2") "tag3"` {
		t.Fatalf("unexpected expansion: %s", got)
	}
	roundTrip, err := Parse(got)
	if err != nil {
		t.Fatalf("expanded query did not parse: %v", err)
	}
	if err := ValidateAST(roundTrip); err != nil {
		t.Fatalf("expanded query did not validate: %v", err)
	}
}

func TestExpandSavedSearchesSupportsNegationAndComposition(t *testing.T) {
	ast, err := Parse("-@saved:foo | @saved:bar")
	if err != nil {
		t.Fatal(err)
	}
	queries := map[string]string{"foo": "a b", "bar": "c | d"}
	if err := ExpandSavedSearches(ast, func(name string) (string, error) { return queries[name], nil }); err != nil {
		t.Fatal(err)
	}
	got := Format(ast)
	if got != `-("a" "b") | ("c" | "d")` {
		t.Fatalf("unexpected expansion: %s", got)
	}
}

func TestExpandSavedSearchesExpandsNestedReferences(t *testing.T) {
	ast, err := Parse("@saved:outer")
	if err != nil {
		t.Fatal(err)
	}
	queries := map[string]string{"outer": "x @saved:inner", "inner": "y | z"}
	if err := ExpandSavedSearches(ast, func(name string) (string, error) { return queries[name], nil }); err != nil {
		t.Fatal(err)
	}
	if got := Format(ast); got != `("x" ("y" | "z"))` {
		t.Fatalf("unexpected nested expansion: %s", got)
	}
}

func TestExpandSavedSearchesTreatsEmptySearchAsAllFiles(t *testing.T) {
	ast, err := Parse("@saved:all tag1 | -@saved:all")
	if err != nil {
		t.Fatal(err)
	}
	if err := ExpandSavedSearches(ast, func(name string) (string, error) { return "", nil }); err != nil {
		t.Fatal(err)
	}
	got := Format(ast)
	if !strings.Contains(got, `"@tagged" | -"@tagged"`) {
		t.Fatalf("empty saved search did not expand to all-files expression: %s", got)
	}
	roundTrip, err := Parse(got)
	if err != nil {
		t.Fatalf("expanded empty saved search did not parse: %v", err)
	}
	if err := ValidateAST(roundTrip); err != nil {
		t.Fatalf("expanded empty saved search did not validate: %v", err)
	}
}

func TestExpandSavedSearchesRejectsCycles(t *testing.T) {
	ast, err := Parse("@saved:a")
	if err != nil {
		t.Fatal(err)
	}
	queries := map[string]string{"a": "@saved:b", "b": "@saved:a"}
	err = ExpandSavedSearches(ast, func(name string) (string, error) { return queries[name], nil })
	if err == nil || !strings.Contains(err.Error(), "saved search cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestSavedMetaTagRequiresName(t *testing.T) {
	if _, err := Parse("@saved:"); err == nil {
		t.Fatal("expected @saved: without a name to be invalid")
	}
}
