package serve

import (
	"context"
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
	return nil, nil
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

func TestBrowseUnfilteredAggregatesReuseExactFileCount(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	library := &aggregateCountingLibrary{}
	server := NewServerWithLibrary(cfg, library)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/files?include_facets=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if library.countFilesCalls != 1 {
		t.Fatalf("expected one exact file count, got %d", library.countFilesCalls)
	}
	if library.libraryCountForQueryCalls != 0 || library.libraryCountCalls != 0 {
		t.Fatalf("unfiltered browse repeated library count: query=%d global=%d", library.libraryCountForQueryCalls, library.libraryCountCalls)
	}
}

func TestBrowseFilteredAggregatesKeepQueryLibraryCount(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	library := &aggregateCountingLibrary{}
	server := NewServerWithLibrary(cfg, library)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/files?query=kind%3Aphoto&include_facets=true", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if library.countFilesCalls != 1 || library.libraryCountForQueryCalls != 1 {
		t.Fatalf("filtered browse should retain both aggregate lookups, count=%d library=%d", library.countFilesCalls, library.libraryCountForQueryCalls)
	}
}
