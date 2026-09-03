package cmd

import (
	"bytes"
	"strings"
	"testing"

	"gooru.local/types"
)

type fakeSavedSearchClient struct {
	createUsername string
	createName     string
	createQuery    string
	createSort     string
	createOrder    string
	searchUsername string
	searchRef      string
}

func (f *fakeSavedSearchClient) ListSavedSearchesForUsername(username string) ([]types.SavedSearch, error) {
	return []types.SavedSearch{{ID: "srch_one", Name: "Favorites", Query: "cat", Sort: "name", Order: "asc"}}, nil
}

func (f *fakeSavedSearchClient) CreateSavedSearchForUsername(username, name, expression, sort, order string) (types.SavedSearch, error) {
	f.createUsername = username
	f.createName = name
	f.createQuery = expression
	f.createSort = sort
	f.createOrder = order
	return types.SavedSearch{ID: "srch_created", Name: name, Query: expression, Sort: sort, Order: order}, nil
}

func (f *fakeSavedSearchClient) SearchSavedSearchForUsername(username, reference string, verbose bool) ([]types.FileInfo, error) {
	f.searchUsername = username
	f.searchRef = reference
	return []types.FileInfo{{Path: "/library/a.jpg"}, {Path: "/library/b.jpg"}}, nil
}

func TestSavedSearchCreateCommandForwardsUserQueryAndSort(t *testing.T) {
	fake := &fakeSavedSearchClient{}
	cmd := newSavedSearchCmd(func() savedSearchClient { return fake })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--user", "Alice", "create", "Favorites", "cat", "dog", "--sort", "modified", "--order", "desc"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute saved-search create: %v", err)
	}
	if fake.createUsername != "Alice" || fake.createName != "Favorites" || fake.createQuery != "cat dog" || fake.createSort != "modified" || fake.createOrder != "desc" {
		t.Fatalf("unexpected create args: %+v", fake)
	}
	if got := strings.TrimSpace(out.String()); got != "srch_created\tFavorites\tcat dog\tmodified\tdesc" {
		t.Fatalf("unexpected create output %q", got)
	}
}

func TestSavedSearchSearchCommandPrintsMatchingPaths(t *testing.T) {
	fake := &fakeSavedSearchClient{}
	cmd := newSavedSearchCmd(func() savedSearchClient { return fake })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--user", "Alice", "search", "Favorites"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute saved-search search: %v", err)
	}
	if fake.searchUsername != "Alice" || fake.searchRef != "Favorites" {
		t.Fatalf("unexpected search args: %+v", fake)
	}
	if got := strings.TrimSpace(out.String()); got != "/library/a.jpg\n/library/b.jpg" {
		t.Fatalf("unexpected search output %q", got)
	}
}

func TestSavedSearchCommandRequiresUser(t *testing.T) {
	fake := &fakeSavedSearchClient{}
	cmd := newSavedSearchCmd(func() savedSearchClient { return fake })
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "required flag") {
		t.Fatalf("expected required --user error, got %v", err)
	}
}
