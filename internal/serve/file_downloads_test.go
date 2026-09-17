package serve

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gooru.local/internal/securekey"
	"gooru.local/types"
)

type downloadTestLibrary struct {
	*snapshotTestLibrary
	batchSizes []int
}

func (l *downloadTestLibrary) GetFilesByPublicIDs(ctx context.Context, ids []string) ([]types.FileInfo, error) {
	l.batchSizes = append(l.batchSizes, len(ids))
	files := make([]types.FileInfo, 0, len(ids))
	for _, id := range ids {
		file, err := l.GetFileByPublicID(ctx, id)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func newDownloadTestServer(t *testing.T, cfg Config, files map[string]types.FileInfo) (*Server, *downloadTestLibrary) {
	t.Helper()
	base := newSnapshotTestLibrary()
	for id, file := range files {
		base.files[id] = file
	}
	library := &downloadTestLibrary{snapshotTestLibrary: base}
	return NewServerWithLibrary(cfg, library), library
}

func createDownloadForTest(t *testing.T, server *Server, body string) fileDownloadCreateResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/file-downloads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create download: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var response fileDownloadCreateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode download response: %v", err)
	}
	return response
}

func readZipForTest(t *testing.T, data []byte) []*zip.File {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	return reader.File
}

func TestBulkDownloadStreamsStoredEntriesAndDisambiguatesNames(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "one")
	secondDir := filepath.Join(root, "two")
	if err := os.MkdirAll(firstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondDir, 0o755); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(firstDir, "photo.jpg")
	secondPath := filepath.Join(secondDir, "photo.jpg")
	if err := os.WriteFile(firstPath, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	server, library := newDownloadTestServer(t, cfg, map[string]types.FileInfo{
		"a": {PublicID: "a", Path: firstPath, Size: 5},
		"b": {PublicID: "b", Path: secondPath, Size: 6},
	})
	download := createDownloadForTest(t, server, `{"file_ids":["a","b"]}`)

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, download.URL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("Content-Type = %q, want application/zip", got)
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("Content-Disposition = %q, want attachment", rec.Header().Get("Content-Disposition"))
	}
	files := readZipForTest(t, rec.Body.Bytes())
	if len(files) != 2 {
		t.Fatalf("zip entries = %d, want 2", len(files))
	}
	if files[0].Name != "photo.jpg" || files[1].Name != "photo (2).jpg" {
		t.Fatalf("zip names = [%q %q]", files[0].Name, files[1].Name)
	}
	for index, want := range []string{"first", "second"} {
		if files[index].Method != zip.Store {
			t.Fatalf("entry %d compression method = %d, want STORE", index, files[index].Method)
		}
		reader, err := files[index].Open()
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("entry %d body = %q, want %q", index, string(got), want)
		}
	}
	if len(library.batchSizes) != 1 || library.batchSizes[0] != 2 {
		t.Fatalf("batch lookups = %v, want [2]", library.batchSizes)
	}
}

func TestBulkDownloadUsesFrozenSelectionWithDeltas(t *testing.T) {
	root := t.TempDir()
	files := make(map[string]types.FileInfo)
	for _, id := range []string{"a", "b", "c"} {
		path := filepath.Join(root, id+".txt")
		if err := os.WriteFile(path, []byte(id), 0o600); err != nil {
			t.Fatal(err)
		}
		files[id] = types.FileInfo{PublicID: id, Path: path, Size: 1, Tags: []string{"hidden"}}
	}
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	server, library := newDownloadTestServer(t, cfg, files)
	snapshot := createSnapshotForTest(t, server, "hidden")

	latePath := filepath.Join(root, "late.txt")
	if err := os.WriteFile(latePath, []byte("late"), 0o600); err != nil {
		t.Fatal(err)
	}
	library.files["late"] = types.FileInfo{PublicID: "late", Path: latePath, Size: 4, Tags: []string{"hidden"}}

	body := `{"selection_id":` + quoteJSON(snapshot.ID) + `,"exclude_file_ids":["b"],"include_file_ids":["late"]}`
	download := createDownloadForTest(t, server, body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, download.URL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	entries := readZipForTest(t, rec.Body.Bytes())
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	if got := strings.Join(names, ","); got != "a.txt,c.txt,late.txt" {
		t.Fatalf("selection archive names = %q, want a.txt,c.txt,late.txt", got)
	}
}

func TestBulkDownloadReadsProtectedOriginals(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "secret.bin")
	plaintext := []byte("private-download-content")
	if err := os.WriteFile(path, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x53}, securekey.Size)
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: root}}
	encryptMediaFixture(t, path, cfg.Encryption.Key)

	server, _ := newDownloadTestServer(t, cfg, map[string]types.FileInfo{
		"secret": {PublicID: "secret", Path: path, Size: int64(len(plaintext))},
	})
	download := createDownloadForTest(t, server, `{"file_ids":["secret"]}`)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, download.URL, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("download: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	entries := readZipForTest(t, rec.Body.Bytes())
	if len(entries) != 1 {
		t.Fatalf("zip entries = %d, want 1", len(entries))
	}
	reader, err := entries[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("protected zip entry = %q, want original plaintext", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != protectedAPICacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, protectedAPICacheControl)
	}
}

func TestBulkDownloadBoundsMetadataLookupBatches(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tiny.bin")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	files := make(map[string]types.FileInfo, fileDownloadLookupBatch+1)
	ids := make([]string, 0, fileDownloadLookupBatch+1)
	for index := 0; index < fileDownloadLookupBatch+1; index++ {
		id := fmt.Sprintf("file-%03d", index)
		ids = append(ids, id)
		files[id] = types.FileInfo{PublicID: id, Path: path, Size: 1}
	}

	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Enabled = false
	server, library := newDownloadTestServer(t, cfg, files)
	rec := httptest.NewRecorder()
	started, err := server.streamFileDownloadArchive(rec, context.Background(), ids)
	if err != nil {
		t.Fatalf("stream archive: %v", err)
	}
	if !started {
		t.Fatal("archive did not start")
	}
	if got := fmt.Sprint(library.batchSizes); got != "[256 1]" {
		t.Fatalf("metadata lookup batches = %s, want [256 1]", got)
	}
	if len(readZipForTest(t, rec.Body.Bytes())) != len(ids) {
		t.Fatalf("zip entry count did not match %d selected files", len(ids))
	}
}

func TestFileDownloadTicketExpiresAndIsOwnerScoped(t *testing.T) {
	store := newFileDownloadStore()
	base := time.Now().UTC()
	store.now = func() time.Time { return base }
	id, err := store.create("owner-a", bulkFileTarget{FileIDs: []string{"a"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.resolve("owner-b", id); !errors.Is(err, errFileDownloadNotFound) {
		t.Fatalf("other owner resolve = %v, want not found", err)
	}
	store.now = func() time.Time { return base.Add(fileDownloadTTL + time.Second) }
	if _, err := store.resolve("owner-a", id); !errors.Is(err, errFileDownloadNotFound) {
		t.Fatalf("expired resolve = %v, want not found", err)
	}
}
