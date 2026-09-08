package serve

import (
	"errors"
	"strings"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestExpandSavedSearchQueryUsesOnlyProvidedUserSearches(t *testing.T) {
	saved := []types.SavedSearch{
		{Name: "Favorites", Query: "rating:5 | starred"},
		{Name: "Everything", Query: ""},
	}

	got, err := expandSavedSearchQuery("@saved:Favorites landscape", saved)
	if err != nil {
		t.Fatal(err)
	}
	if got != `("rating:5" | "starred") "landscape"` {
		t.Fatalf("unexpected expansion: %s", got)
	}

	got, err = expandSavedSearchQuery("@saved:Everything landscape", saved)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"@tagged" | -"@tagged"`) {
		t.Fatalf("empty saved search did not preserve all-files semantics: %s", got)
	}

	_, err = expandSavedSearchQuery("@saved:OtherUser", saved)
	if !errors.Is(err, core.ErrInvalidQuery) {
		t.Fatalf("missing user-scoped saved search should be invalid query, got %v", err)
	}
}

func TestSavedSearchNameSuggestionsAreScopedAndPreserveNegation(t *testing.T) {
	saved := []types.SavedSearch{
		{Name: "Favorites"},
		{Name: "Family"},
		{Name: "Work"},
	}

	got := savedSearchNameSuggestions("@saved:fa", saved, 20)
	if len(got) != 2 || got[0].Name != "@saved:Favorites" || got[1].Name != "@saved:Family" {
		t.Fatalf("unexpected saved search suggestions: %#v", got)
	}

	got = savedSearchNameSuggestions("-@saved:fav", saved, 20)
	if len(got) != 1 || got[0].Name != "-@saved:Favorites" {
		t.Fatalf("negated completion was not preserved: %#v", got)
	}

	if got := savedSearchNameSuggestions("ordinary", saved, 20); len(got) != 0 {
		t.Fatalf("ordinary prefix should not produce saved search suggestions: %#v", got)
	}
}
