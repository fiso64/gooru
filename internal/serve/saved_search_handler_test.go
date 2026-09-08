package serve

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gooru.local/types"
)

type savedSearchBrowseLibrary struct {
	savedByUser         map[string][]types.SavedSearch
	savedUserIDs        []string
	listQueries         []string
	facetQueries        []string
	countQueries        []string
	libraryCountQueries []string
	facetErr            error
}

func (l *savedSearchBrowseLibrary) ListFiles(context.Context, string) ([]types.FileInfo, error) {
	return nil, nil
}

func (l *savedSearchBrowseLibrary) GetFile(context.Context, int64) (types.FileInfo, error) {
	return types.FileInfo{}, ErrNotFound
}

func (l *savedSearchBrowseLibrary) ListTags(context.Context, bool, int) ([]TagDTO, error) {
	return nil, nil
}

func (l *savedSearchBrowseLibrary) ListFilesSearch(_ context.Context, query string, _ Page, _, _ string) (PageResult[types.FileInfo], error) {
	l.listQueries = append(l.listQueries, query)
	return PageResult[types.FileInfo]{Items: []types.FileInfo{}}, nil
}

func (l *savedSearchBrowseLibrary) LibraryCount(context.Context) (int, error) {
	return 0, nil
}

func (l *savedSearchBrowseLibrary) KindFacets(_ context.Context, query string) ([]FacetValueDTO, error) {
	l.facetQueries = append(l.facetQueries, query)
	if l.facetErr != nil {
		return nil, l.facetErr
	}
	return []FacetValueDTO{{Value: "photo", Count: 0}}, nil
}

func (l *savedSearchBrowseLibrary) TagSuggestions(context.Context, string, string, int) ([]TagDTO, error) {
	return nil, nil
}

func (l *savedSearchBrowseLibrary) TagNamespaces(context.Context) ([]string, error) {
	return nil, nil
}

func (l *savedSearchBrowseLibrary) FileMetadata(context.Context, int64) (MediaMetadata, error) {
	return MediaMetadata{}, nil
}

func (l *savedSearchBrowseLibrary) CountFiles(_ context.Context, query string) (int, error) {
	l.countQueries = append(l.countQueries, query)
	return 0, nil
}

func (l *savedSearchBrowseLibrary) LibraryCountForQuery(_ context.Context, query string) (int, error) {
	l.libraryCountQueries = append(l.libraryCountQueries, query)
	return 0, nil
}

func (l *savedSearchBrowseLibrary) ListSavedSearches(_ context.Context, userID string) ([]types.SavedSearch, error) {
	l.savedUserIDs = append(l.savedUserIDs, userID)
	return l.savedByUser[userID], nil
}

func (l *savedSearchBrowseLibrary) CreateSavedSearch(context.Context, string, savedSearchRequest) (types.SavedSearch, error) {
	return types.SavedSearch{}, nil
}

func (l *savedSearchBrowseLibrary) UpdateSavedSearch(context.Context, string, string, savedSearchRequest) (types.SavedSearch, error) {
	return types.SavedSearch{}, nil
}

func (l *savedSearchBrowseLibrary) ReorderSavedSearches(context.Context, string, []string) error {
	return nil
}

func (l *savedSearchBrowseLibrary) DeleteSavedSearch(context.Context, string, string) (bool, error) {
	return false, nil
}

func requestWithUser(req *http.Request, userID string) *http.Request {
	ctx := context.WithValue(req.Context(), authContextKey{}, AuthSession{User: User{ID: userID}})
	return req.WithContext(ctx)
}

func TestBrowseSavedSearchExpandsOnceForAuthenticatedUserAndAggregates(t *testing.T) {
	library := &savedSearchBrowseLibrary{
		savedByUser: map[string][]types.SavedSearch{
			"user-a": {{Name: "Favorites", Query: "rating:5 | starred"}},
		},
		facetErr: errors.New("facet backend unavailable"),
	}
	server := &Server{library: library}
	rec := httptest.NewRecorder()
	req := requestWithUser(httptest.NewRequest(http.MethodGet, "/api/v1/files", nil), "user-a")

	server.handleListFilesRequest(rec, req, fileSearchRequest{Query: "@saved:Favorites landscape", IncludeFacets: true})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(library.savedUserIDs) != 1 || library.savedUserIDs[0] != "user-a" {
		t.Fatalf("expected exactly one saved-search lookup for user-a, got %#v", library.savedUserIDs)
	}
	want := `("rating:5" | "starred") "landscape"`
	if len(library.listQueries) != 1 || library.listQueries[0] != want {
		t.Fatalf("list path did not receive expanded query: %#v", library.listQueries)
	}
	if len(library.facetQueries) != 1 || library.facetQueries[0] != want {
		t.Fatalf("facet path did not receive expanded query: %#v", library.facetQueries)
	}
	if len(library.countQueries) != 1 || library.countQueries[0] != want {
		t.Fatalf("count path did not receive expanded query: %#v", library.countQueries)
	}
	if len(library.libraryCountQueries) != 1 || library.libraryCountQueries[0] != want {
		t.Fatalf("library-count path did not receive expanded query: %#v", library.libraryCountQueries)
	}
}

func TestBrowseSavedSearchRequiresAuthenticatedRequestContext(t *testing.T) {
	library := &savedSearchBrowseLibrary{savedByUser: map[string][]types.SavedSearch{
		"user-a": {{Name: "Favorites", Query: "rating:5"}},
	}}
	server := &Server{library: library}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)

	server.handleListFilesRequest(rec, req, fileSearchRequest{Query: "@saved:Favorites"})

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_query")
	if len(library.savedUserIDs) != 0 || len(library.listQueries) != 0 {
		t.Fatalf("unauthenticated saved search should not reach library lookup/listing: users=%#v queries=%#v", library.savedUserIDs, library.listQueries)
	}
}

func TestBrowseSavedSearchCannotResolveAnotherUsersSearch(t *testing.T) {
	library := &savedSearchBrowseLibrary{savedByUser: map[string][]types.SavedSearch{
		"user-a": {{Name: "Favorites", Query: "rating:5"}},
		"user-b": {{Name: "Private", Query: "secret"}},
	}}
	server := &Server{library: library}
	rec := httptest.NewRecorder()
	req := requestWithUser(httptest.NewRequest(http.MethodGet, "/api/v1/files", nil), "user-a")

	server.handleListFilesRequest(rec, req, fileSearchRequest{Query: "@saved:Private"})

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_query")
	if len(library.savedUserIDs) != 1 || library.savedUserIDs[0] != "user-a" {
		t.Fatalf("expected lookup to stay scoped to user-a, got %#v", library.savedUserIDs)
	}
	if len(library.listQueries) != 0 {
		t.Fatalf("cross-user saved search should not reach listing: %#v", library.listQueries)
	}
}

func TestBrowseEmptySavedSearchCompletesWithAllFilesSemantics(t *testing.T) {
	library := &savedSearchBrowseLibrary{savedByUser: map[string][]types.SavedSearch{
		"user-a": {{Name: "Everything", Query: ""}},
	}}
	server := &Server{library: library}
	rec := httptest.NewRecorder()
	req := requestWithUser(httptest.NewRequest(http.MethodGet, "/api/v1/files", nil), "user-a")

	server.handleListFilesRequest(rec, req, fileSearchRequest{Query: "@saved:Everything"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(library.listQueries) != 1 || !strings.Contains(library.listQueries[0], `"@tagged" | -"@tagged"`) {
		t.Fatalf("empty saved search did not preserve all-files semantics at handler boundary: %#v", library.listQueries)
	}
}

func TestSavedSearchSuggestionsUseAuthenticatedUsersSearchesAndLimit(t *testing.T) {
	library := &savedSearchBrowseLibrary{savedByUser: map[string][]types.SavedSearch{
		"user-a": {{Name: "Favorites"}, {Name: "Family"}},
		"user-b": {{Name: "Favorite Secret"}},
	}}
	server := &Server{library: library}
	rec := httptest.NewRecorder()
	req := requestWithUser(httptest.NewRequest(http.MethodGet, "/api/v1/search/suggestions?q=@saved:fa&limit=1", nil), "user-a")

	server.handleSearchSuggestions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(library.savedUserIDs) != 1 || library.savedUserIDs[0] != "user-a" {
		t.Fatalf("expected suggestions lookup for user-a only, got %#v", library.savedUserIDs)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"name":"@saved:Favorites"`) || strings.Contains(body, "Favorite Secret") || strings.Contains(body, "@saved:Family") {
		t.Fatalf("unexpected scoped/limited saved search suggestions: %s", body)
	}
}
