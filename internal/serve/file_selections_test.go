package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"gooru.local/types"
)

type snapshotTestLibrary struct {
	mu             sync.Mutex
	files          map[string]types.FileInfo
	removed        []string
	lastTagRequest TagMutationRequest
}

func newSnapshotTestLibrary(ids ...string) *snapshotTestLibrary {
	library := &snapshotTestLibrary{files: make(map[string]types.FileInfo)}
	for _, id := range ids {
		library.add(id, "hidden")
	}
	return library
}

func (l *snapshotTestLibrary) add(id string, tags ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.files[id] = types.FileInfo{PublicID: id, Path: "/tmp/" + id + ".jpg", Hash: "hash-" + id, Tags: append([]string(nil), tags...)}
}

func (l *snapshotTestLibrary) ListFiles(_ context.Context, expression string) ([]types.FileInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	files := make([]types.FileInfo, 0, len(l.files))
	for _, file := range l.files {
		if expression != "*" && !containsString(file.Tags, expression) {
			continue
		}
		files = append(files, file)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].PublicID < files[j].PublicID })
	return files, nil
}

func (l *snapshotTestLibrary) GetFile(_ context.Context, _ int64) (types.FileInfo, error) {
	return types.FileInfo{}, ErrNotFound
}

func (l *snapshotTestLibrary) ListTags(context.Context, bool, int) ([]TagDTO, error) { return nil, nil }
func (l *snapshotTestLibrary) PublicFileID(file types.FileInfo) string                  { return file.PublicID }

func (l *snapshotTestLibrary) GetFileByPublicID(_ context.Context, id string) (types.FileInfo, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	file, ok := l.files[id]
	if !ok {
		return types.FileInfo{}, ErrNotFound
	}
	return file, nil
}

func (l *snapshotTestLibrary) DeleteFileByPublicID(_ context.Context, id string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.files[id]; !ok {
		return false, nil
	}
	delete(l.files, id)
	l.removed = append(l.removed, id)
	return true, nil
}

func (l *snapshotTestLibrary) MutateTags(_ context.Context, operation TagOperation, request TagMutationRequest) (TagMutationResponse, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lastTagRequest = request
	return TagMutationResponse{Operation: operation, MatchedFiles: len(request.FileIDs), AffectedCount: len(request.FileIDs)}, nil
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func newSnapshotTestServer(t *testing.T, library *snapshotTestLibrary) *Server {
	t.Helper()
	cfg := DefaultConfig(t.TempDir() + "/gooru.db")
	cfg.Auth.Enabled = false
	return NewServerWithLibrary(cfg, library)
}

func createSnapshotForTest(t *testing.T, server *Server, query string) fileSelectionCreateResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-selections", bytes.NewBufferString(`{"query":`+quoteJSON(query)+`}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create snapshot: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var response fileSelectionCreateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode snapshot response: %v", err)
	}
	return response
}

func quoteJSON(value string) string {
	body, _ := json.Marshal(value)
	return string(body)
}

func TestFileSelectionSnapshotDoesNotGrowWithLiveQuery(t *testing.T) {
	library := newSnapshotTestLibrary("a", "b")
	server := newSnapshotTestServer(t, library)
	snapshot := createSnapshotForTest(t, server, "hidden")
	if snapshot.Count != 2 {
		t.Fatalf("expected snapshot count 2, got %d", snapshot.Count)
	}

	library.add("c", "hidden")
	library.add("d", "hidden")

	body := []byte(`{"mode":"untrack","selection_id":` + quoteJSON(snapshot.ID) + `}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot untrack: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := append([]string(nil), library.removed...); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("snapshot mutated the wrong files: %v", got)
	}
	if _, err := library.GetFileByPublicID(context.Background(), "c"); err != nil {
		t.Fatalf("file added after snapshot should remain: %v", err)
	}
	if _, err := library.GetFileByPublicID(context.Background(), "d"); err != nil {
		t.Fatalf("file added after snapshot should remain: %v", err)
	}
}

func TestFileSelectionSnapshotSupportsExplicitIncludeAndExclude(t *testing.T) {
	library := newSnapshotTestLibrary("a", "b")
	server := newSnapshotTestServer(t, library)
	snapshot := createSnapshotForTest(t, server, "hidden")
	library.add("c", "hidden")

	body := []byte(`{"mode":"untrack","selection_id":` + quoteJSON(snapshot.ID) + `,"include_file_ids":["c"],"exclude_file_ids":["b"]}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot include/exclude: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := append([]string(nil), library.removed...); len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("unexpected include/exclude target set: %v", got)
	}
	if _, err := library.GetFileByPublicID(context.Background(), "b"); err != nil {
		t.Fatalf("excluded snapshot member should remain: %v", err)
	}
}

func TestFileSelectionMembershipRejectsLaterMatches(t *testing.T) {
	library := newSnapshotTestLibrary("a", "b")
	server := newSnapshotTestServer(t, library)
	snapshot := createSnapshotForTest(t, server, "hidden")
	library.add("c", "hidden")

	body := bytes.NewBufferString(`{"file_ids":["a","c"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-selections/"+snapshot.ID+"/members", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("membership: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response fileSelectionMembersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode membership response: %v", err)
	}
	if len(response.FileIDs) != 1 || response.FileIDs[0] != "a" {
		t.Fatalf("expected only original member a, got %v", response.FileIDs)
	}
}

func TestFileSelectionSnapshotBindsTagMutationBeforeLaterMatches(t *testing.T) {
	library := newSnapshotTestLibrary("a", "b")
	server := newSnapshotTestServer(t, library)
	snapshot := createSnapshotForTest(t, server, "hidden")
	library.add("c", "hidden")

	body := []byte(`{"selection_id":` + quoteJSON(snapshot.ID) + `,"tags":["reviewed"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/tags", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot tag: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := append([]string(nil), library.lastTagRequest.FileIDs...)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("tag mutation was not bound to snapshot: %v", got)
	}
	if library.lastTagRequest.Query != "" || library.lastTagRequest.SelectionID != "" {
		t.Fatalf("tag mutator should receive explicit resolved IDs, got %+v", library.lastTagRequest)
	}
}

func TestExpiredFileSelectionFailsClosed(t *testing.T) {
	library := newSnapshotTestLibrary("a")
	server := newSnapshotTestServer(t, library)
	base := time.Now().UTC()
	server.fileSelections.now = func() time.Time { return base }
	snapshot := createSnapshotForTest(t, server, "hidden")
	server.fileSelections.now = func() time.Time { return base.Add(fileSelectionTTL + time.Second) }

	body := []byte(`{"mode":"untrack","selection_id":` + quoteJSON(snapshot.ID) + `}`)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertAPIError(t, rec, http.StatusGone, "selection_expired")
	if len(library.removed) != 0 {
		t.Fatalf("expired selection must not mutate files: %v", library.removed)
	}
}
