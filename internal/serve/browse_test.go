package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestFileIDRoundTrip(t *testing.T) {
	id := EncodeFileID(42)
	if id == "42" || id == "loc:42" {
		t.Fatalf("file id is not opaque: %q", id)
	}
	got, err := DecodeFileID(id)
	if err != nil {
		t.Fatalf("DecodeFileID: %v", err)
	}
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestBrowseRoutesRequireBearerToken(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Token = "secret"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)

	NewServerWithLibrary(cfg, emptyLibrary{}).Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusUnauthorized, "unauthorized")
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
	if page.NextPageToken == "" {
		t.Fatal("expected next page token")
	}
	if page.Files[0].MediaKind != "image" {
		t.Fatalf("expected image media kind, got %q", page.Files[0].MediaKind)
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

func newTestBrowseServer(t *testing.T) (*Server, func()) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
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
	cfg.Auth.Token = "secret"
	return NewServerWithLibrary(cfg, NewGooruLibrary(client, false)), func() { _ = client.Close() }
}

func writeTestFile(t *testing.T, dir string, name string, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	return path
}

func authedRequest(method string, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer secret")
	return req
}

type emptyLibrary struct{}

func (emptyLibrary) ListFiles(_ context.Context, _ string) ([]types.FileInfo, error) {
	return nil, nil
}

func (emptyLibrary) GetFile(_ context.Context, _ int64) (types.FileInfo, error) {
	return types.FileInfo{}, ErrNotFound
}

func (emptyLibrary) ListTags(_ context.Context, _ bool) ([]TagDTO, error) {
	return nil, nil
}
