from pathlib import Path


def replace_once(path: str, old: str, new: str) -> None:
    file = Path(path)
    text = file.read_text()
    if old in text:
        file.write_text(text.replace(old, new, 1))


replace_once(
    "types/types.go",
    '''type PageCursor struct {\n\tSort  string `json:"sort"`\n\tOrder string `json:"order"`\n\tID    int64  `json:"id"`\n}\n''',
    '''type PageCursor struct {\n\tSort   string `json:"sort"`\n\tOrder  string `json:"order"`\n\tID     int64  `json:"id"`\n\tOffset int    `json:"offset,omitempty"`\n}\n''',
)

pagination = Path("internal/serve/pagination.go")
text = pagination.read_text()
text = text.replace(
    "return Page{Limit: limit, Cursor: &cursor}, nil",
    "return Page{Limit: limit, Offset: cursor.Offset, Cursor: &cursor}, nil",
)
old = '''func CursorPageToken(sort string, order string, id int64) string {\n\tif id <= 0 {\n\t\treturn ""\n\t}\n\tpayload, err := json.Marshal(types.PageCursor{Sort: sort, Order: order, ID: id})\n\tif err != nil {\n\t\treturn ""\n\t}\n\treturn base64.RawURLEncoding.EncodeToString(payload)\n}\n'''
new = '''func CursorPageToken(sort string, order string, id int64) string {\n\treturn CursorPageTokenAtOffset(sort, order, id, 0)\n}\n\nfunc CursorPageTokenAtOffset(sort string, order string, id int64, offset int) string {\n\tif id <= 0 || offset < 0 {\n\t\treturn ""\n\t}\n\tpayload, err := json.Marshal(types.PageCursor{Sort: sort, Order: order, ID: id, Offset: offset})\n\tif err != nil {\n\t\treturn ""\n\t}\n\treturn base64.RawURLEncoding.EncodeToString(payload)\n}\n'''
if old in text:
    text = text.replace(old, new, 1)
text = text.replace(
    '''\tif cursor.ID <= 0 {\n\t\treturn nil, fmt.Errorf("page_token is invalid")\n\t}\n''',
    '''\tif cursor.ID <= 0 || cursor.Offset < 0 {\n\t\treturn nil, fmt.Errorf("page_token is invalid")\n\t}\n''',
    1,
)
pagination.write_text(text)

replace_once(
    "internal/serve/browse.go",
    '''\tif len(result.Items) > page.Limit {\n\t\tresult.Items = result.Items[:page.Limit]\n\t\tif cursor != nil {\n\t\t\tlast := result.Items[len(result.Items)-1]\n\t\t\tresult.NextPageToken = CursorPageToken(sort, order, last.ID)\n\t\t} else {\n\t\t\tresult.NextPageToken = NextPageToken(page.Offset, page.Limit, page.Limit)\n\t\t}\n\t}\n''',
    '''\tif len(result.Items) > page.Limit {\n\t\tresult.Items = result.Items[:page.Limit]\n\t\tlast := result.Items[len(result.Items)-1]\n\t\tresult.NextPageToken = CursorPageTokenAtOffset(sort, order, last.ID, page.Offset+page.Limit)\n\t}\n''',
)

tests = Path("internal/serve/browse_test.go")
text = tests.read_text()
old = '''\tif !strings.HasPrefix(string(decodedToken), "offset:") {\n\t\tt.Fatalf("file search should use offset token for bidirectional UI paging, got %q", string(decodedToken))\n\t}\n'''
new = '''\tif strings.HasPrefix(string(decodedToken), "offset:") {\n\t\tt.Fatalf("file search should advance with a stable cursor token, got %q", string(decodedToken))\n\t}\n\tparsedPage, err := ParsePage("1", page.NextPageToken)\n\tif err != nil || parsedPage.Cursor == nil || parsedPage.Offset != 1 {\n\t\tt.Fatalf("expected cursor token with retained logical offset, page=%+v err=%v", parsedPage, err)\n\t}\n'''
if old in text:
    text = text.replace(old, new, 1)
marker = "func TestBrowseIncludesCountsFacetsAndCachedMetadata(t *testing.T) {"
if "TestBrowseCursorRemainsStableWhenEarlierRowsAreInserted" not in text:
    test = '''func TestBrowseCursorRemainsStableWhenEarlierRowsAreInserted(t *testing.T) {\n\tserver, cleanup := newTestBrowseServer(t)\n\tdefer cleanup()\n\n\tfirstRec := httptest.NewRecorder()\n\tserver.Handler().ServeHTTP(firstRec, authedRequest(http.MethodGet, "/api/v1/files?sort=name&order=asc&limit=1"))\n\tif firstRec.Code != http.StatusOK {\n\t\tt.Fatalf("expected first page 200, got %d: %s", firstRec.Code, firstRec.Body.String())\n\t}\n\tvar first FileListResponse\n\tif err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {\n\t\tt.Fatalf("decode first page: %v", err)\n\t}\n\tif len(first.Files) != 1 || first.NextPageToken == "" {\n\t\tt.Fatalf("unexpected first page: %+v", first)\n\t}\n\tif first.Files[0].Name != "a.jpg" {\n\t\tt.Fatalf("expected a.jpg first, got %+v", first.Files)\n\t}\n\n\tclient := server.library.(*GooruLibrary).client\n\tinserted := writeTestFile(t, t.TempDir(), "0-new.jpg", "new earlier file")\n\tif _, err := client.TagFiles([]string{inserted}, nil, nil, false); err != nil {\n\t\tt.Fatalf("insert file ahead of cursor: %v", err)\n\t}\n\n\tnextRec := httptest.NewRecorder()\n\tnextURL := "/api/v1/files?sort=name&order=asc&limit=1&page_token=" + url.QueryEscape(first.NextPageToken)\n\tserver.Handler().ServeHTTP(nextRec, authedRequest(http.MethodGet, nextURL))\n\tif nextRec.Code != http.StatusOK {\n\t\tt.Fatalf("expected next page 200, got %d: %s", nextRec.Code, nextRec.Body.String())\n\t}\n\tvar next FileListResponse\n\tif err := json.Unmarshal(nextRec.Body.Bytes(), &next); err != nil {\n\t\tt.Fatalf("decode next page: %v", err)\n\t}\n\tif len(next.Files) != 1 || next.Files[0].Name != "b.png" {\n\t\tt.Fatalf("expected stable cursor to continue at b.png after earlier insertion, first=%+v next=%+v", first.Files, next.Files)\n\t}\n\tif next.Files[0].ID == first.Files[0].ID {\n\t\tt.Fatalf("pagination duplicated the boundary row after earlier insertion: %+v", next.Files[0])\n\t}\n\tif next.PreviousPageToken == "" {\n\t\tt.Fatal("cursor page should retain offset metadata for backward navigation")\n\t}\n}\n\n'''
    text = text.replace(marker, test + marker, 1)
tests.write_text(text)
