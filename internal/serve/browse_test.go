package serve

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/internal/database"
	"gooru.local/types"
)

func TestFileIDRoundTrip(t *testing.T) {
	id := fallbackPublicFileID(42)
	if id == "42" || id == "loc:42" || strings.Contains(id, "loc:") {
		t.Fatalf("file id is not opaque: %q", id)
	}
	got, err := fallbackResolveFileID(id)
	if err != nil {
		t.Fatalf("resolve fallback file id: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestFileDTOReportsViewerSupportFromBackendMediaKind(t *testing.T) {
	server := &Server{cfg: DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))}
	image := server.fileDTO(context.Background(), types.FileInfo{Path: filepath.Join(t.TempDir(), "image.jpg")}, false)
	if image.MediaKind != "photo" || image.ViewerSupport != "supported" {
		t.Fatalf("expected supported photo viewer state, got kind=%q support=%q", image.MediaKind, image.ViewerSupport)
	}
	text := server.fileDTO(context.Background(), types.FileInfo{Path: filepath.Join(t.TempDir(), "notes.txt")}, false)
	if text.MediaKind != "other" || text.ViewerSupport != "unsupported_media_type" {
		t.Fatalf("expected unsupported text viewer state, got kind=%q support=%q", text.MediaKind, text.ViewerSupport)
	}
}

func TestBrowseRoutesRequireSession(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServerWithLibrary(cfg, emptyLibrary{})
	attachTestAuth(t, server)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusUnauthorized, "unauthorized")
}

func TestBrowseInvalidQueryReturnsBadRequest(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files?query=(")
	library := errorLibrary{listFilesErr: fmt.Errorf("%w: could not parse query", core.ErrInvalidQuery)}

	NewServerWithLibrary(cfg, library).Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_query")
}

func TestBrowseServiceErrorReturnsInternalError(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	rec := httptest.NewRecorder()
	req := authedRequest(http.MethodGet, "/api/v1/files")
	library := errorLibrary{listFilesErr: errors.New("database exploded")}

	NewServerWithLibrary(cfg, library).Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusInternalServerError, "internal_error")
	if strings.Contains(rec.Body.String(), "database exploded") {
		t.Fatalf("internal error leaked service details: %s", rec.Body.String())
	}
}

func TestBrowseFilesAndDetailsUseOpaqueIDs(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	first := authedRequest(http.MethodGet, "/api/v1/files?query=kind:image&limit=1")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, first)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(page.Files) != 1 {
		t.Fatalf("expected one file, got %d", len(page.Files))
	}
	if page.Files[0].ID == "" || page.Files[0].ID == page.Files[0].Path {
		t.Fatalf("expected opaque file id, got %+v", page.Files[0])
	}
	if page.Files[0].Path != "" {
		t.Fatalf("path should be hidden by default, got %q", page.Files[0].Path)
	}
	if page.Files[0].SafeDisplayPath == "" || filepath.IsAbs(page.Files[0].SafeDisplayPath) || !strings.Contains(page.Files[0].SafeDisplayPath, page.Files[0].Name) {
		t.Fatalf("expected safe display path without absolute path leak, got %+v", page.Files[0])
	}
	if page.NextPageToken == "" {
		t.Fatal("expected next page token")
	}
	decodedToken, err := base64.RawURLEncoding.DecodeString(page.NextPageToken)
	if err != nil {
		t.Fatalf("decode next page token: %v", err)
	}
	if strings.HasPrefix(string(decodedToken), "offset:") {
		t.Fatalf("file search should advance with a stable cursor token, got %q", string(decodedToken))
	}
	parsedPage, err := ParsePage("1", page.NextPageToken)
	if err != nil || parsedPage.Cursor == nil || parsedPage.Offset != 1 {
		t.Fatalf("expected cursor token with retained logical offset, page=%+v err=%v", parsedPage, err)
	}
	if strings.HasPrefix(page.Files[0].ID, "loc:") {
		t.Fatalf("file id exposed storage prefix: %q", page.Files[0].ID)
	}
	if decodedID, err := base64.RawURLEncoding.DecodeString(page.Files[0].ID); err == nil && strings.Contains(string(decodedID), "loc:") {
		t.Fatalf("file id is reversible storage id: %q", string(decodedID))
	}
	storedFile, err := server.getFileByPublicID(context.Background(), page.Files[0].ID)
	if err != nil {
		t.Fatalf("resolve first file id: %v", err)
	}
	if strings.Contains(string(decodedToken), storedFile.Path) || strings.Contains(string(decodedToken), page.Files[0].Name) {
		t.Fatalf("cursor token leaked stored path data: %q", string(decodedToken))
	}
	if page.Files[0].MediaKind != "photo" {
		t.Fatalf("expected photo media kind, got %q", page.Files[0].MediaKind)
	}
	if page.Files[0].AddedAt.IsZero() || page.Files[0].AddedAt.UnixMilli() != storedFile.AddedAt {
		t.Fatalf("expected added_at from stored location, dto=%v stored=%d", page.Files[0].AddedAt, storedFile.AddedAt)
	}

	nextReq := authedRequest(http.MethodGet, "/api/v1/files?query=kind:image&limit=1&page_token="+page.NextPageToken)
	nextRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(nextRec, nextReq)
	if nextRec.Code != http.StatusOK {
		t.Fatalf("expected next page 200, got %d: %s", nextRec.Code, nextRec.Body.String())
	}
	var nextPage FileListResponse
	if err := json.Unmarshal(nextRec.Body.Bytes(), &nextPage); err != nil {
		t.Fatalf("decode next response: %v", err)
	}
	if len(nextPage.Files) != 1 {
		t.Fatalf("expected one file on next page, got %d", len(nextPage.Files))
	}
	if nextPage.NextPageToken != "" {
		t.Fatalf("did not expect trailing next page token, got %q", nextPage.NextPageToken)
	}
	if nextPage.PreviousPageToken == "" {
		t.Fatal("expected previous page token on later page")
	}
	if nextPage.LibraryCount != 0 || len(nextPage.Facets.Kind) != 0 {
		t.Fatalf("later pages should not include aggregate metadata without include_facets, got library=%d facets=%+v", nextPage.LibraryCount, nextPage.Facets)
	}

	detailReq := authedRequest(http.MethodGet, "/api/v1/files/"+page.Files[0].ID)
	detailRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
	var detail FileDTO
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detail.ID != page.Files[0].ID || detail.ContentID == "" || detail.MediaURLs.Content == "" {
		t.Fatalf("unexpected detail DTO: %+v", detail)
	}
}

func TestTagsIncludeLibrarySummaryWithoutListingFiles(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/tags?counts=true"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response TagListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tags response: %v", err)
	}
	if response.LibraryCount != 3 {
		t.Fatalf("expected library count 3, got %d", response.LibraryCount)
	}
	counts := facetCounts(response.Facets.Kind)
	if counts["photo"] == 0 || counts["other"] == 0 {
		t.Fatalf("expected global kind facets in tags response, got %+v", response.Facets.Kind)
	}
}

func TestBrowseDefaultsToAddedNewestAndSupportsAddedCursor(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	if got := normalizeFileSort(""); got != "added" {
		t.Fatalf("expected default sort added, got %q", got)
	}
	if got := normalizeSortOrder(""); got != "desc" {
		t.Fatalf("expected default order desc, got %q", got)
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/files?limit=3"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected default list 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode default list: %v", err)
	}
	if len(page.Files) < 2 {
		t.Fatalf("expected multiple files, got %d", len(page.Files))
	}
	for i := 1; i < len(page.Files); i++ {
		if page.Files[i-1].AddedAt.Before(page.Files[i].AddedAt) {
			t.Fatalf("default list is not added-at descending: %v before %v", page.Files[i-1].AddedAt, page.Files[i].AddedAt)
		}
	}

	firstRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRec, authedRequest(http.MethodGet, "/api/v1/files?sort=added&order=desc&limit=1"))
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first added page 200, got %d: %s", firstRec.Code, firstRec.Body.String())
	}
	var first FileListResponse
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first added page: %v", err)
	}
	if len(first.Files) != 1 {
		t.Fatalf("expected one first-page file, got %d", len(first.Files))
	}
	stored, err := server.getFileByPublicID(context.Background(), first.Files[0].ID)
	if err != nil {
		t.Fatalf("resolve first added-sort file: %v", err)
	}
	cursor := CursorPageToken("added", "desc", stored.ID)
	secondRec := httptest.NewRecorder()
	secondURL := "/api/v1/files?sort=added&order=desc&limit=1&page_token=" + url.QueryEscape(cursor)
	server.Handler().ServeHTTP(secondRec, authedRequest(http.MethodGet, secondURL))
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected added cursor page 200, got %d: %s", secondRec.Code, secondRec.Body.String())
	}
	var second FileListResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second added page: %v", err)
	}
	if len(second.Files) != 1 || second.Files[0].ID == first.Files[0].ID {
		t.Fatalf("expected cursor to advance to another file, first=%+v second=%+v", first.Files, second.Files)
	}
}

func TestBrowseCursorRemainsStableWhenEarlierRowsAreInserted(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	firstRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(firstRec, authedRequest(http.MethodGet, "/api/v1/files?sort=name&order=asc&limit=1"))
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first page 200, got %d: %s", firstRec.Code, firstRec.Body.String())
	}
	var first FileListResponse
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Files) != 1 || first.NextPageToken == "" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	if first.Files[0].Name != "a.jpg" {
		t.Fatalf("expected a.jpg first, got %+v", first.Files)
	}

	client := server.library.(*GooruLibrary).client
	inserted := writeTestFile(t, t.TempDir(), "0-new.jpg", "new earlier file")
	if _, err := client.TagFiles([]string{inserted}, nil, nil, false); err != nil {
		t.Fatalf("insert file ahead of cursor: %v", err)
	}

	nextRec := httptest.NewRecorder()
	nextURL := "/api/v1/files?sort=name&order=asc&limit=1&page_token=" + url.QueryEscape(first.NextPageToken)
	server.Handler().ServeHTTP(nextRec, authedRequest(http.MethodGet, nextURL))
	if nextRec.Code != http.StatusOK {
		t.Fatalf("expected next page 200, got %d: %s", nextRec.Code, nextRec.Body.String())
	}
	var next FileListResponse
	if err := json.Unmarshal(nextRec.Body.Bytes(), &next); err != nil {
		t.Fatalf("decode next page: %v", err)
	}
	if len(next.Files) != 1 || next.Files[0].Name != "b.png" {
		t.Fatalf("expected stable cursor to continue at b.png after earlier insertion, first=%+v next=%+v", first.Files, next.Files)
	}
	if next.Files[0].ID == first.Files[0].ID {
		t.Fatalf("pagination duplicated the boundary row after earlier insertion: %+v", next.Files[0])
	}
	if next.PreviousPageToken == "" {
		t.Fatal("cursor page should retain offset metadata for backward navigation")
	}
}

func TestBrowseIncludesCountsFacetsAndCachedMetadata(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/files?limit=2&sort=size&order=desc&include_facets=true"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(page.Files) != 2 || page.TotalCount != 3 || page.LibraryCount != 3 {
		t.Fatalf("unexpected paged totals: %+v", page)
	}
	if len(page.Facets.Kind) == 0 {
		t.Fatalf("expected kind facets, got %+v", page.Facets)
	}
	textRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(textRec, authedRequest(http.MethodGet, "/api/v1/files?query=kind:text&include_facets=true"))
	if textRec.Code != http.StatusOK {
		t.Fatalf("expected text search 200, got %d: %s", textRec.Code, textRec.Body.String())
	}
	var textPage FileListResponse
	if err := json.Unmarshal(textRec.Body.Bytes(), &textPage); err != nil {
		t.Fatalf("decode text response: %v", err)
	}
	textFacets := facetCounts(textPage.Facets.Kind)
	if textFacets["other"] != 1 || textFacets["photo"] != 0 {
		t.Fatalf("expected query-scoped kind facets, got %+v", textFacets)
	}
	file, err := server.getFileByPublicID(context.Background(), page.Files[0].ID)
	if err != nil {
		t.Fatalf("resolve file id: %v", err)
	}
	width, height := 640, 480
	if err := server.library.(*GooruLibrary).client.UpsertMediaMetadata(types.MediaMetadata{
		LocationID:  file.ID,
		MediaKind:   "video",
		MimeType:    "video/mp4",
		ImageWidth:  &width,
		ImageHeight: &height,
	}); err != nil {
		t.Fatalf("upsert metadata: %v", err)
	}

	detailRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(detailRec, authedRequest(http.MethodGet, "/api/v1/files/"+page.Files[0].ID))
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d: %s", detailRec.Code, detailRec.Body.String())
	}
	var detail FileDTO
	if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Metadata.ImageWidth == nil || *detail.Metadata.ImageWidth != width {
		t.Fatalf("expected cached metadata, got %+v", detail.Metadata)
	}
	if detail.MediaKind != "video" || detail.MediaType != "video/mp4" {
		t.Fatalf("expected cached media type/kind, got %+v", detail)
	}
	if detail.MediaURLs.Download == "" {
		t.Fatalf("expected download URL, got %+v", detail.MediaURLs)
	}

	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, authedRequest(http.MethodGet, "/api/v1/files?include_facets=true&limit=3"))
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listPage FileListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listPage); err != nil {
		t.Fatalf("decode metadata list: %v", err)
	}
	var foundCached bool
	for _, file := range listPage.Files {
		if file.ID == page.Files[0].ID {
			foundCached = file.Metadata.ImageWidth != nil && file.MediaKind == "video" && file.MediaType == "video/mp4"
		}
	}
	if !foundCached {
		t.Fatalf("expected list response to use cached metadata, got %+v", listPage.Files)
	}
	if facetCounts(listPage.Facets.Kind)["video"] == 0 {
		t.Fatalf("expected metadata-backed video facet, got %+v", listPage.Facets.Kind)
	}
}

func TestBrowseBareQueryDoesNotSearchFilename(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	dir := t.TempDir()
	match := writeTestFile(t, dir, "needle-file.jpg", "same duplicate bytes")
	other := writeTestFile(t, dir, "other-file.jpg", "same duplicate bytes")
	if _, err := server.library.(*GooruLibrary).client.TagFiles([]string{match, other}, []string{"album:dupes"}, nil, false); err != nil {
		t.Fatalf("tag duplicate files: %v", err)
	}

	rec := httptest.NewRecorder()
	target := "/api/v1/files?query=" + url.QueryEscape("needle-file") + "&limit=10"
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, target))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(page.Files) != 0 {
		t.Fatalf("bare query should not match filenames, got %+v", page.Files)
	}
}

func TestBrowseFilesCanExposePathsWhenConfigured(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()
	server.cfg.Server.ExposePaths = true

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/files?query=kind:image&limit=1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(page.Files) != 1 {
		t.Fatalf("expected one file, got %d", len(page.Files))
	}
	if page.Files[0].Path == "" {
		t.Fatal("expected path when server.expose_paths is true")
	}
	if page.Files[0].SafeDisplayPath == "" || page.Files[0].SafeDisplayPath == page.Files[0].Path {
		t.Fatalf("expected separate safe display path, got %+v", page.Files[0])
	}
}

func TestSearchSuggestionsNamespacesDeleteAndDownload(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	suggestions := httptest.NewRecorder()
	server.Handler().ServeHTTP(suggestions, authedRequest(http.MethodGet, "/api/v1/search/suggestions?q=kind&limit=5"))
	if suggestions.Code != http.StatusOK {
		t.Fatalf("expected suggestions 200, got %d: %s", suggestions.Code, suggestions.Body.String())
	}
	var suggestionResponse SuggestionsResponse
	if err := json.Unmarshal(suggestions.Body.Bytes(), &suggestionResponse); err != nil {
		t.Fatalf("decode suggestions: %v", err)
	}
	if len(suggestionResponse.Items) == 0 || suggestionResponse.Items[0].Name == "" {
		t.Fatalf("expected suggestions, got %+v", suggestionResponse)
	}
	suggestionNames := tagNames(suggestionResponse.Items)
	if !containsString(suggestionNames, "kind:") {
		t.Fatalf("expected namespace suggestion, got %+v", suggestionNames)
	}

	values := httptest.NewRecorder()
	server.Handler().ServeHTTP(values, authedRequest(http.MethodGet, "/api/v1/search/suggestions?q=kind:&existing=kind:image&limit=5"))
	if values.Code != http.StatusOK {
		t.Fatalf("expected value suggestions 200, got %d: %s", values.Code, values.Body.String())
	}
	var valueResponse SuggestionsResponse
	if err := json.Unmarshal(values.Body.Bytes(), &valueResponse); err != nil {
		t.Fatalf("decode value suggestions: %v", err)
	}
	valueNames := tagNames(valueResponse.Items)
	if containsString(valueNames, "kind:image") || !containsString(valueNames, "kind:text") {
		t.Fatalf("expected existing filter and namespace values, got %+v", valueNames)
	}

	namespaces := httptest.NewRecorder()
	server.Handler().ServeHTTP(namespaces, authedRequest(http.MethodGet, "/api/v1/tags/namespaces"))
	if namespaces.Code != http.StatusOK {
		t.Fatalf("expected namespaces 200, got %d: %s", namespaces.Code, namespaces.Body.String())
	}
	var namespaceResponse NamespacesResponse
	if err := json.Unmarshal(namespaces.Body.Bytes(), &namespaceResponse); err != nil {
		t.Fatalf("decode namespaces: %v", err)
	}
	if !containsString(namespaceResponse.Items, "kind") {
		t.Fatalf("expected kind namespace, got %+v", namespaceResponse.Items)
	}

	page := listTestFiles(t, server, "kind:image", 1)
	fileID := page.Files[0].ID
	download := httptest.NewRecorder()
	server.Handler().ServeHTTP(download, authedRequest(http.MethodGet, "/api/v1/files/"+fileID+"/download"))
	if download.Code != http.StatusOK {
		t.Fatalf("expected download 200, got %d: %s", download.Code, download.Body.String())
	}
	if got := download.Header().Get("Content-Disposition"); !strings.Contains(got, "attachment") {
		t.Fatalf("expected attachment content disposition, got %q", got)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+fileID, bytes.NewBufferString(`{"mode":"untrack"}`))
	deleteReq.Header.Set("Content-Type", "application/json")
	deleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
	missing := httptest.NewRecorder()
	server.Handler().ServeHTTP(missing, authedRequest(http.MethodGet, "/api/v1/files/"+fileID))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected deleted file 404, got %d: %s", missing.Code, missing.Body.String())
	}
}

func TestDeleteFileRejectsMalformedJSON(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	page := listTestFiles(t, server, "kind:image", 1)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+page.Files[0].ID, bytes.NewBufferString(`{`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestSavedSearchCRUDRequiresAuthenticatedUser(t *testing.T) {
	server, auth, cleanup := newAuthenticatedBrowseServer(t)
	defer cleanup()

	unauth := httptest.NewRecorder()
	server.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/v1/saved-searches", nil))
	assertAPIError(t, unauth, http.StatusUnauthorized, "unauthorized")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/saved-searches", bytes.NewBufferString(`{"name":"Images","query":"kind:image","sort":"modified","order":"desc"}`))
	createReq.Header.Set("Content-Type", "application/json")
	addAuthCookie(createReq, server.cfg, auth)
	addCSRF(createReq, auth)
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created SavedSearchDTO
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode saved search: %v", err)
	}
	if created.ID == "" || created.Query != "kind:image" || created.Sort != "modified" || created.Order != "desc" {
		t.Fatalf("unexpected saved search: %+v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/saved-searches", nil)
	addAuthCookie(listReq, server.cfg, auth)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listed SavedSearchesResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode saved search list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != created.ID {
		t.Fatalf("unexpected saved search list: %+v", listed)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/v1/saved-searches/"+created.ID, bytes.NewBufferString(`{"name":"Text","query":"kind:text","sort":"name","order":"asc"}`))
	updateReq.Header.Set("Content-Type", "application/json")
	addAuthCookie(updateReq, server.cfg, auth)
	addCSRF(updateReq, auth)
	updateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected update 200, got %d: %s", updateRec.Code, updateRec.Body.String())
	}

	otherUser, err := server.auth.CreateAdmin(context.Background(), "otheradmin", "correct horse")
	if err != nil {
		t.Fatalf("create other admin: %v", err)
	}
	otherAuth, err := server.auth.Login(context.Background(), otherUser.Username, "correct horse")
	if err != nil {
		t.Fatalf("login other admin: %v", err)
	}
	otherUpdateReq := httptest.NewRequest(http.MethodPut, "/api/v1/saved-searches/"+created.ID, bytes.NewBufferString(`{"name":"Other","query":"kind:image","sort":"name","order":"asc"}`))
	otherUpdateReq.Header.Set("Content-Type", "application/json")
	addAuthCookie(otherUpdateReq, server.cfg, otherAuth)
	addCSRF(otherUpdateReq, otherAuth)
	otherUpdateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(otherUpdateRec, otherUpdateReq)
	assertAPIError(t, otherUpdateRec, http.StatusNotFound, "not_found")

	listAfterReq := httptest.NewRequest(http.MethodGet, "/api/v1/saved-searches", nil)
	addAuthCookie(listAfterReq, server.cfg, auth)
	listAfterRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listAfterRec, listAfterReq)
	if listAfterRec.Code != http.StatusOK {
		t.Fatalf("expected owner list 200, got %d: %s", listAfterRec.Code, listAfterRec.Body.String())
	}
	var listedAfter SavedSearchesResponse
	if err := json.Unmarshal(listAfterRec.Body.Bytes(), &listedAfter); err != nil {
		t.Fatalf("decode owner saved searches: %v", err)
	}
	if len(listedAfter.Items) != 1 || listedAfter.Items[0].Name != "Text" {
		t.Fatalf("cross-user update changed owner saved search: %+v", listedAfter)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/v1/saved-searches/"+created.ID, nil)
	addAuthCookie(deleteReq, server.cfg, auth)
	addCSRF(deleteReq, auth)
	deleteRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}
}

func TestFileDTOMetadataFallsBackWithoutBreakingRoute(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServerWithLibrary(cfg, emptyLibrary{})
	imagePath := writePNGImage(t)

	dto := server.fileDTO(context.Background(), types.FileInfo{ID: 99, Path: imagePath, Hash: "hash", Size: 10}, true)
	if dto.Metadata.ImageWidth == nil || *dto.Metadata.ImageWidth != 32 {
		t.Fatalf("expected image width metadata, got %+v", dto.Metadata)
	}
	if dto.Metadata.ImageHeight == nil || *dto.Metadata.ImageHeight != 24 {
		t.Fatalf("expected image height metadata, got %+v", dto.Metadata)
	}

	dto = server.fileDTO(context.Background(), types.FileInfo{ID: 100, Path: filepath.Join(t.TempDir(), "missing.jpg"), Hash: "hash", Size: 10}, true)
	if dto.ID == "" || dto.Metadata.ImageWidth != nil || dto.Metadata.ImageHeight != nil {
		t.Fatalf("metadata failure should not block DTO fallback, got %+v", dto)
	}
}

func TestPaginateInMemoryBounds(t *testing.T) {
	page, err := ParsePage("2", "")
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	first := PaginateInMemory([]int{1, 2, 3}, page)
	if fmt.Sprint(first.Items) != "[1 2]" || first.NextPageToken == "" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	next, err := ParsePage("2", first.NextPageToken)
	if err != nil {
		t.Fatalf("ParsePage next: %v", err)
	}
	last := PaginateInMemory([]int{1, 2, 3}, next)
	if fmt.Sprint(last.Items) != "[3]" || last.NextPageToken != "" {
		t.Fatalf("unexpected last page: %+v", last)
	}
	empty := PaginateInMemory([]int{1}, Page{Limit: 2, Offset: 99})
	if len(empty.Items) != 0 || empty.NextPageToken != "" {
		t.Fatalf("unexpected empty page: %+v", empty)
	}
}

func TestParsePageRejectsInvalidTokensAndLimits(t *testing.T) {
	cases := []struct {
		name     string
		limit    string
		token    string
		wantErr  string
		wantPage Page
	}{
		{name: "bad limit", limit: "nope", wantErr: "limit must be a positive integer"},
		{name: "zero limit", limit: "0", wantErr: "limit must be a positive integer"},
		{name: "negative limit", limit: "-1", wantErr: "limit must be a positive integer"},
		{name: "bad token encoding", token: "%%%not-base64", wantErr: "page_token is invalid"},
		{name: "bad token prefix", token: mustPageToken("page:5"), wantErr: "page_token is invalid"},
		{name: "negative offset", token: mustPageToken("offset:-1"), wantErr: "page_token is invalid"},
		{name: "non-integer offset", token: mustPageToken("offset:abc"), wantErr: "page_token is invalid"},
		{name: "max limit clamps", limit: "9999", wantPage: Page{Limit: MaxPageLimit, Offset: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := ParsePage(tc.limit, tc.token)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got page=%+v err=%v", tc.wantErr, page, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePage failed: %v", err)
			}
			if page != tc.wantPage {
				t.Fatalf("expected page %+v, got %+v", tc.wantPage, page)
			}
		})
	}
}

func TestListTagsWithCounts(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/tags?counts=true"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response TagListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tags response: %v", err)
	}
	counts := make(map[string]int)
	for _, tag := range response.Tags {
		if tag.Count != nil {
			counts[tag.Name] = *tag.Count
		}
	}
	if counts["kind:image"] != 2 {
		t.Fatalf("expected kind:image count 2, got %d in %+v", counts["kind:image"], counts)
	}
}

func TestListTagsWithoutCountsHonorsLimit(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/tags?limit=1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response TagListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode tags response: %v", err)
	}
	if len(response.Tags) != 1 {
		t.Fatalf("expected exactly one limited tag, got %+v", response.Tags)
	}
	if response.Tags[0].Count != nil {
		t.Fatalf("expected counts=false response without counts, got %+v", response.Tags[0])
	}
}

func TestMediaKindForAudioType(t *testing.T) {
	if got := mediaKindForType("audio/mpeg"); got != "audio" {
		t.Fatalf("expected audio media kind, got %q", got)
	}
}

func newTestBrowseServer(t *testing.T) (*Server, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	return server, func() { _ = client.Close() }
}

func newAuthenticatedBrowseServer(t *testing.T) (*Server, AuthSession, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	cfg := server.cfg
	cfg.Auth.Enabled = true
	server.cfg = cfg

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open auth db: %v", err)
	}
	if err := database.RunMigrations(db); err != nil {
		t.Fatalf("run auth migrations: %v", err)
	}
	store := NewAuthStore(db, cfg.Auth.SessionTTL)
	if _, err := store.CreateAdmin(context.Background(), "testadmin", "correct horse"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	auth, err := store.Login(context.Background(), "testadmin", "correct horse")
	if err != nil {
		t.Fatalf("login admin: %v", err)
	}
	server.SetAuthStore(store)
	return server, auth, func() {
		_ = db.Close()
		_ = client.Close()
	}
}

func newTestBrowseServerAt(t *testing.T, dir string, dbPath string) (*Server, *core.Client) {
	t.Helper()
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	imageA := writeTestFile(t, dir, "a.jpg", "fake image a")
	imageB := writeTestFile(t, dir, "b.png", "fake image b")
	other := writeTestFile(t, dir, "notes.txt", "notes")
	if _, err := client.TagFiles([]string{imageA, imageB}, []string{"kind:image"}, nil, false); err != nil {
		t.Fatalf("tag images: %v", err)
	}
	if _, err := client.TagFiles([]string{other}, []string{"kind:text"}, nil, false); err != nil {
		t.Fatalf("tag text: %v", err)
	}

	cfg := DefaultConfig(dbPath)
	cfg.Auth.Enabled = false
	return NewServerWithLibrary(cfg, NewGooruLibrary(client, false)), client
}

func listTestFiles(t *testing.T, server *Server, query string, limit int) FileListResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	target := fmt.Sprintf("/api/v1/files?query=%s&limit=%d", url.QueryEscape(query), limit)
	server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, target))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var page FileListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(page.Files) == 0 {
		t.Fatalf("expected files in %+v", page)
	}
	return page
}

func tagNames(tags []TagDTO) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		out = append(out, tag.Name)
	}
	return out
}

func facetCounts(facets []FacetValueDTO) map[string]int {
	out := make(map[string]int, len(facets))
	for _, facet := range facets {
		out[facet.Value] = facet.Count
	}
	return out
}

func writeTestFile(t *testing.T, dir string, name string, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}

func mustPageToken(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func authedRequest(method string, target string) *http.Request {
	return httptest.NewRequest(method, target, nil)
}

type emptyLibrary struct{}

func (emptyLibrary) ListFiles(_ context.Context, _ string) ([]types.FileInfo, error) {
	return nil, nil
}

func (emptyLibrary) GetFile(_ context.Context, _ int64) (types.FileInfo, error) {
	return types.FileInfo{}, ErrNotFound
}

func (emptyLibrary) ListTags(_ context.Context, _ bool, _ int) ([]TagDTO, error) {
	return nil, nil
}

func (emptyLibrary) PublicFileID(file types.FileInfo) string {
	return fallbackPublicFileID(file.ID)
}

func (l emptyLibrary) GetFileByPublicID(ctx context.Context, id string) (types.FileInfo, error) {
	locationID, err := fallbackResolveFileID(id)
	if err != nil {
		return types.FileInfo{}, err
	}
	return l.GetFile(ctx, locationID)
}

func (emptyLibrary) DeleteFileByPublicID(_ context.Context, _ string) (bool, error) {
	return false, ErrNotFound
}

type errorLibrary struct {
	listFilesErr error
}

func (l errorLibrary) ListFiles(_ context.Context, _ string) ([]types.FileInfo, error) {
	return nil, l.listFilesErr
}

func (errorLibrary) GetFile(_ context.Context, _ int64) (types.FileInfo, error) {
	return types.FileInfo{}, ErrNotFound
}

func (errorLibrary) ListTags(_ context.Context, _ bool, _ int) ([]TagDTO, error) {
	return nil, nil
}

func TestNewTagCompletionUsesComponentPrefixesThroughHTTP(t *testing.T) {
 server, cleanup := newTestBrowseServer(t)
 defer cleanup()
 page := listTestFiles(t, server, "kind:image", 1)
 added := httptest.NewRecorder()
 server.Handler().ServeHTTP(added, authedJSONRequest(http.MethodPost, "/api/v1/files/tags", `{"file_ids":["`+page.Files[0].ID+`"],"tags":["test_name:value_more"]}`))
 if added.Code != http.StatusOK {
  t.Fatalf("tag mutation failed: %d: %s", added.Code, added.Body.String())
 }
 for _, prefix := range []string{"test", "name", "val", "more"} {
  rec := httptest.NewRecorder()
  server.Handler().ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/search/suggestions?q="+prefix+"&limit=10"))
  if rec.Code != http.StatusOK { t.Fatalf("lookup %q: %d: %s", prefix, rec.Code, rec.Body.String()) }
  var response SuggestionsResponse
  if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil { t.Fatal(err) }
  if !containsString(tagNames(response.Items), "test_name:value_more") {
   t.Fatalf("newly added tag absent from %q suggestions: %+v", prefix, response.Items)
  }
 }
}
