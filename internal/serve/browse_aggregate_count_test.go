package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

type aggregateCountingLibrary struct {
	countFilesCalls           int
	libraryCountCalls         int
	libraryCountForQueryCalls int
	kindFacetsCalls           int
	kindFacets                []FacetValueDTO
	kindFacetsErr             error
}

func (l *aggregateCountingLibrary) ListFiles(context.Context, string) ([]types.FileInfo, error) {
	return nil, nil
}

func (l *aggregateCountingLibrary) GetFile(context.Context, int64) (types.FileInfo, error) {
	return types.FileInfo{}, ErrNotFound
}

func (l *aggregateCountingLibrary) ListTags(context.Context, bool, int) ([]TagDTO, error) {
	return nil, nil
}

func (l *aggregateCountingLibrary) ListFilesSearch(context.Context, string, Page, string, string) (PageResult[types.FileInfo], error) {
	return PageResult[types.FileInfo]{}, nil
}

func (l *aggregateCountingLibrary) LibraryCount(context.Context) (int, error) {
	l.libraryCountCalls++
	return 99, nil
}

func (l *aggregateCountingLibrary) LibraryCountForQuery(context.Context, string) (int, error) {
	l.libraryCountForQueryCalls++
	return 99, nil
}

func (l *aggregateCountingLibrary) CountFiles(context.Context, string) (int, error) {
	l.countFilesCalls++
	return 42, nil
}

func (l *aggregateCountingLibrary) KindFacets(context.Context, string) ([]FacetValueDTO, error) {
	l.kindFacetsCalls++
	return l.kindFacets, l.kindFacetsErr
}

func (l *aggregateCountingLibrary) TagSuggestions(context.Context, string, string, int) ([]TagDTO, error) {
	return nil, nil
}

func (l *aggregateCountingLibrary) TagNamespaces(context.Context) ([]string, error) {
	return nil, nil
}

func (l *aggregateCountingLibrary) FileMetadata(context.Context, int64) (MediaMetadata, error) {
	return MediaMetadata{}, nil
}

func TestBrowseUnfilteredAggregatesReuseKindFacetTotal(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	library := &aggregateCountingLibrary{kindFacets: []FacetValueDTO{{Value: "photo", Count: 40}, {Value: "video", Count: 2}}}
	server := NewServerWithLibrary(cfg, library)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/files?include_facets=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if library.kindFacetsCalls != 1 || library.countFilesCalls != 0 {
		t.Fatalf("expected facet total without exact count query, facets=%d count=%d", library.kindFacetsCalls, library.countFilesCalls)
	}
	if library.libraryCountForQueryCalls != 0 || library.libraryCountCalls != 0 {
		t.Fatalf("unfiltered browse repeated library count: query=%d global=%d", library.libraryCountForQueryCalls, library.libraryCountCalls)
	}
}

func TestBrowseFilteredAggregatesReuseKindFacetTotal(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	library := &aggregateCountingLibrary{kindFacets: []FacetValueDTO{{Value: "photo", Count: 42}}}
	server := NewServerWithLibrary(cfg, library)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/files?query=kind%3Aphoto&include_facets=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if library.kindFacetsCalls != 1 || library.countFilesCalls != 0 || library.libraryCountForQueryCalls != 1 {
		t.Fatalf("filtered browse should reuse facets and retain query library count, facets=%d count=%d library=%d", library.kindFacetsCalls, library.countFilesCalls, library.libraryCountForQueryCalls)
	}
}

func TestBrowseAggregatesFallBackToExactCountWhenFacetsFail(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	library := &aggregateCountingLibrary{kindFacetsErr: errors.New("facet query failed")}
	server := NewServerWithLibrary(cfg, library)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/files?query=kind%3Aphoto&include_facets=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if library.kindFacetsCalls != 1 || library.countFilesCalls != 1 || library.libraryCountForQueryCalls != 1 {
		t.Fatalf("facet failure should fall back to exact count, facets=%d count=%d library=%d", library.kindFacetsCalls, library.countFilesCalls, library.libraryCountForQueryCalls)
	}
}

func TestTagsAggregatesReuseKindFacetTotal(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	library := &aggregateCountingLibrary{kindFacets: []FacetValueDTO{{Value: "photo", Count: 40}, {Value: "video", Count: 2}}}
	server := NewServerWithLibrary(cfg, library)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tags?counts=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if library.kindFacetsCalls != 1 || library.libraryCountCalls != 0 {
		t.Fatalf("tags should reuse root kind facets without a second library count, facets=%d library=%d", library.kindFacetsCalls, library.libraryCountCalls)
	}
	var response TagListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.LibraryCount != 42 {
		t.Fatalf("library_count = %d, want 42 from facet partition", response.LibraryCount)
	}
}

func TestTagsAggregatesFallBackToLibraryCountWhenFacetsFail(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	library := &aggregateCountingLibrary{kindFacetsErr: errors.New("facet query failed")}
	server := NewServerWithLibrary(cfg, library)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tags?counts=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if library.kindFacetsCalls != 1 || library.libraryCountCalls != 1 {
		t.Fatalf("facet failure should fall back to one library count, facets=%d library=%d", library.kindFacetsCalls, library.libraryCountCalls)
	}
	var response TagListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.LibraryCount != 99 {
		t.Fatalf("library_count = %d, want fallback 99", response.LibraryCount)
	}
}
