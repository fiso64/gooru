from pathlib import Path


def replace(path, old, new, count=1):
    p = Path(path)
    text = p.read_text()
    actual = text.count(old)
    if actual != count:
        raise SystemExit(f"{path}: expected {count} occurrences, found {actual}: {old!r}")
    p.write_text(text.replace(old, new))


replace(
    "internal/database/database.go",
    'switch sort {\n\tcase "modified":\n\t\treturn "l.mod_time"',
    'switch sort {\n\tcase "added":\n\t\treturn "l.added_at"\n\tcase "modified":\n\t\treturn "l.mod_time"',
)
replace("internal/database/database.go", 'case "modified", "size":', 'case "added", "modified", "size":')
replace(
    "internal/serve/browse.go",
    'case "modified", "name", "size", "kind":\n\t\treturn strings.ToLower(strings.TrimSpace(value))\n\tdefault:\n\t\treturn "name"',
    'case "added", "modified", "name", "size", "kind":\n\t\treturn strings.ToLower(strings.TrimSpace(value))\n\tdefault:\n\t\treturn "added"',
)
replace(
    "internal/serve/browse.go",
    'func normalizeSortOrder(value string) string {\n\tif strings.EqualFold(strings.TrimSpace(value), "desc") {\n\t\treturn "desc"\n\t}\n\treturn "asc"\n}',
    'func normalizeSortOrder(value string) string {\n\tif strings.EqualFold(strings.TrimSpace(value), "asc") {\n\t\treturn "asc"\n\t}\n\treturn "desc"\n}',
)
replace("docs/openapi.yaml", "enum: [name, modified, size, kind]\n            default: name", "enum: [added, name, modified, size, kind]\n            default: added")
replace("docs/openapi.yaml", "enum: [asc, desc]\n            default: asc", "enum: [asc, desc]\n            default: desc")
replace("frontend/src/lib/queries/files.ts", "export type FileSort = 'modified' | 'name' | 'size' | 'kind';", "export type FileSort = 'added' | 'modified' | 'name' | 'size' | 'kind';")
replace("frontend/src/lib/queries/files.ts", "sort: 'modified',\n      order: 'desc',", "sort: 'added',\n      order: 'desc',", 2)
replace("frontend/src/lib/utils/appRoute.ts", "sort: 'modified',\n  order: 'desc',", "sort: 'added',\n  order: 'desc',")
replace("frontend/src/lib/utils/appRoute.ts", "const fileSorts = new Set<FileSort>(['modified', 'name', 'size', 'kind']);", "const fileSorts = new Set<FileSort>(['added', 'modified', 'name', 'size', 'kind']);")
replace("frontend/src/lib/utils/appRoute.test.ts", "expect(searchForLibraryURLState({ query: '', kind: '', sort: 'modified', order: 'desc', fileID: '' })).toBe('');", "expect(searchForLibraryURLState({ query: '', kind: '', sort: 'added', order: 'desc', fileID: '' })).toBe('');")
replace("frontend/src/lib/utils/appRoute.test.ts", "sort: 'modified',\n      order: 'desc',", "sort: 'added',\n      order: 'desc',")
replace("frontend/src/lib/components/AuthenticatedApp.svelte", "{#each [{ value: 'modified', label: 'Modified' }, { value: 'name', label: 'Name' }, { value: 'size', label: 'Size' }] as option}", "{#each [{ value: 'added', label: 'Added' }, { value: 'name', label: 'Name' }, { value: 'size', label: 'Size' }] as option}")

marker = "func TestBrowseIncludesCountsFacetsAndCachedMetadata(t *testing.T) {"
test = r'''func TestBrowseDefaultsToAddedNewestAndSupportsAddedCursor(t *testing.T) {
\tserver, cleanup := newTestBrowseServer(t)
\tdefer cleanup()

\tif got := normalizeFileSort(""); got != "added" {
\t\tt.Fatalf("expected default sort added, got %q", got)
\t}
\tif got := normalizeSortOrder(""); got != "desc" {
\t\tt.Fatalf("expected default order desc, got %q", got)
\t}

\trec := httptest.NewRecorder()
\tserver.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/files?limit=3"))
\tif rec.Code != http.StatusOK {
\t\tt.Fatalf("expected default list 200, got %d: %s", rec.Code, rec.Body.String())
\t}
\tvar page FileListResponse
\tif err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
\t\tt.Fatalf("decode default list: %v", err)
\t}
\tif len(page.Files) < 2 {
\t\tt.Fatalf("expected multiple files, got %d", len(page.Files))
\t}
\tfor i := 1; i < len(page.Files); i++ {
\t\tif page.Files[i-1].AddedAt.Before(page.Files[i].AddedAt) {
\t\t\tt.Fatalf("default list is not added-at descending: %v before %v", page.Files[i-1].AddedAt, page.Files[i].AddedAt)
\t\t}
\t}

\tfirstRec := httptest.NewRecorder()
\tserver.Handler().ServeHTTP(firstRec, authedRequest(http.MethodGet, "/api/v1/files?sort=added&order=desc&limit=1"))
\tif firstRec.Code != http.StatusOK {
\t\tt.Fatalf("expected first added page 200, got %d: %s", firstRec.Code, firstRec.Body.String())
\t}
\tvar first FileListResponse
\tif err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
\t\tt.Fatalf("decode first added page: %v", err)
\t}
\tif len(first.Files) != 1 {
\t\tt.Fatalf("expected one first-page file, got %d", len(first.Files))
\t}
\tstored, err := server.getFileByPublicID(context.Background(), first.Files[0].ID)
\tif err != nil {
\t\tt.Fatalf("resolve first added-sort file: %v", err)
\t}
\tcursor := CursorPageToken("added", "desc", stored.ID)
\tsecondRec := httptest.NewRecorder()
\tsecondURL := "/api/v1/files?sort=added&order=desc&limit=1&page_token=" + url.QueryEscape(cursor)
\tserver.Handler().ServeHTTP(secondRec, authedRequest(http.MethodGet, secondURL))
\tif secondRec.Code != http.StatusOK {
\t\tt.Fatalf("expected added cursor page 200, got %d: %s", secondRec.Code, secondRec.Body.String())
\t}
\tvar second FileListResponse
\tif err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
\t\tt.Fatalf("decode second added page: %v", err)
\t}
\tif len(second.Files) != 1 || second.Files[0].ID == first.Files[0].ID {
\t\tt.Fatalf("expected cursor to advance to another file, first=%+v second=%+v", first.Files, second.Files)
\t}
}

'''
p = Path("internal/serve/browse_test.go")
text = p.read_text()
if text.count(marker) != 1:
    raise SystemExit("browse_test.go: insertion marker mismatch")
p.write_text(text.replace(marker, test.replace("\\t", "\t") + marker))
