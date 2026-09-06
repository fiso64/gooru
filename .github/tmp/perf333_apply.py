from pathlib import Path

browse = Path('internal/serve/browse.go')
text = browse.read_text()
old = '''\tincludeAggregates := req.IncludeFacets
\tif search, ok := s.library.(SearchLibrary); ok && includeAggregates {
\t\tif total, err := s.countFiles(r.Context(), queryText); err == nil {
\t\t\tresponse.TotalCount = total
\t\t}
\t\tif counter, ok := s.library.(QueryLibraryCount); ok {
\t\t\tif total, err := counter.LibraryCountForQuery(r.Context(), queryText); err == nil {
\t\t\t\tresponse.LibraryCount = total
\t\t\t}
\t\t} else if total, err := search.LibraryCount(r.Context()); err == nil {
\t\t\tresponse.LibraryCount = total
\t\t}
\t\tif kind, err := search.KindFacets(r.Context(), queryText); err == nil {
\t\t\tresponse.Facets.Kind = kind
\t\t}
\t}
'''
new = '''\tincludeAggregates := req.IncludeFacets
\tif search, ok := s.library.(SearchLibrary); ok && includeAggregates {
\t\tlibraryCountKnown := false
\t\tif total, err := s.countFiles(r.Context(), queryText); err == nil {
\t\t\tresponse.TotalCount = total
\t\t\tif strings.TrimSpace(queryText) == "" {
\t\t\t\t// An unfiltered location count is the library count. Reuse the exact
\t\t\t\t// result instead of issuing the same COUNT(*) again on large libraries.
\t\t\t\tresponse.LibraryCount = total
\t\t\t\tlibraryCountKnown = true
\t\t\t}
\t\t}
\t\tif !libraryCountKnown {
\t\t\tif counter, ok := s.library.(QueryLibraryCount); ok {
\t\t\t\tif total, err := counter.LibraryCountForQuery(r.Context(), queryText); err == nil {
\t\t\t\t\tresponse.LibraryCount = total
\t\t\t\t}
\t\t\t} else if total, err := search.LibraryCount(r.Context()); err == nil {
\t\t\t\tresponse.LibraryCount = total
\t\t\t}
\t\t}
\t\tif kind, err := search.KindFacets(r.Context(), queryText); err == nil {
\t\t\tresponse.Facets.Kind = kind
\t\t}
\t}
'''
if old not in text:
    raise SystemExit('target aggregate block not found')
browse.write_text(text.replace(old, new, 1))

Path('internal/serve/browse_aggregate_count_test.go').write_text('''package serve

import (
\t"context"
\t"net/http"
\t"net/http/httptest"
\t"path/filepath"
\t"testing"

\t"gooru.local/types"
)

type aggregateCountingLibrary struct {
\tcountFilesCalls           int
\tlibraryCountCalls         int
\tlibraryCountForQueryCalls int
}

func (l *aggregateCountingLibrary) ListFiles(context.Context, string) ([]types.FileInfo, error) { return nil, nil }
func (l *aggregateCountingLibrary) GetFile(context.Context, int64) (types.FileInfo, error) { return types.FileInfo{}, ErrNotFound }
func (l *aggregateCountingLibrary) ListTags(context.Context, bool, int) ([]TagDTO, error) { return nil, nil }
func (l *aggregateCountingLibrary) ListFilesSearch(context.Context, string, Page, string, string) (PageResult[types.FileInfo], error) { return PageResult[types.FileInfo]{}, nil }
func (l *aggregateCountingLibrary) LibraryCount(context.Context) (int, error) { l.libraryCountCalls++; return 99, nil }
func (l *aggregateCountingLibrary) LibraryCountForQuery(context.Context, string) (int, error) { l.libraryCountForQueryCalls++; return 99, nil }
func (l *aggregateCountingLibrary) CountFiles(context.Context, string) (int, error) { l.countFilesCalls++; return 42, nil }
func (l *aggregateCountingLibrary) KindFacets(context.Context, string) ([]FacetValueDTO, error) { return nil, nil }
func (l *aggregateCountingLibrary) TagSuggestions(context.Context, string, string, int) ([]TagDTO, error) { return nil, nil }
func (l *aggregateCountingLibrary) TagNamespaces(context.Context) ([]string, error) { return nil, nil }
func (l *aggregateCountingLibrary) FileMetadata(context.Context, int64) (MediaMetadata, error) { return MediaMetadata{}, nil }

func TestBrowseUnfilteredAggregatesReuseExactFileCount(t *testing.T) {
\tcfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
\tcfg.Auth.Enabled = false
\tlibrary := &aggregateCountingLibrary{}
\tserver := NewServerWithLibrary(cfg, library)
\trec := httptest.NewRecorder()
\tserver.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/files?include_facets=true", nil))
\tif rec.Code != http.StatusOK { t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String()) }
\tif library.countFilesCalls != 1 { t.Fatalf("expected one exact file count, got %d", library.countFilesCalls) }
\tif library.libraryCountForQueryCalls != 0 || library.libraryCountCalls != 0 { t.Fatalf("unfiltered browse repeated library count: query=%d global=%d", library.libraryCountForQueryCalls, library.libraryCountCalls) }
}

func TestBrowseFilteredAggregatesKeepQueryLibraryCount(t *testing.T) {
\tcfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
\tcfg.Auth.Enabled = false
\tlibrary := &aggregateCountingLibrary{}
\tserver := NewServerWithLibrary(cfg, library)
\trec := httptest.NewRecorder()
\tserver.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/files?query=kind%3Aphoto&include_facets=true", nil))
\tif rec.Code != http.StatusOK { t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String()) }
\tif library.countFilesCalls != 1 || library.libraryCountForQueryCalls != 1 { t.Fatalf("filtered browse should retain both aggregate lookups, count=%d library=%d", library.countFilesCalls, library.libraryCountForQueryCalls) }
}
''')
