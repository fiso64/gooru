package serve

import (
	"bytes"
	"context"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/securekey"
	"gooru.local/types"
)

func TestProtectedUploadImportFailureRestoresOriginalRetryPathAfterRename(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	writeInitializedTestDB(t, dbPath, types.StrategyFull)
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	uploadDir := filepath.Join(dir, "uploads")
	cfg := DefaultConfig(filepath.Join(dir, "serve.db"))
	cfg.Auth.Enabled = false
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x71}, securekey.Size)
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
	library := NewGooruLibrary(client, false)
	server := NewServerWithLibrary(cfg, library)
	startTestBackgroundRuntime(t, server, client, "test-protected-retry")

	logicalPath := filepath.Join(uploadDir, "same.png")
	first := tinyPNG(t, 2, 2, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	seed := httptest.NewRecorder()
	server.Handler().ServeHTTP(seed, uploadBinaryRequestWithSourceModTime(t, "same.png", first, time.Time{}))
	if seed.Code != http.StatusOK {
		t.Fatalf("seed protected upload status = %d: %s", seed.Code, seed.Body.String())
	}
	if _, err := client.GetFileInfoByPath(logicalPath); err != nil {
		t.Fatalf("seed protected upload was not tracked: %v", err)
	}
	if _, err := os.Stat(logicalPath); !os.IsNotExist(err) {
		t.Fatalf("seed protected logical path still exists: %v", err)
	}

	second := tinyPNG(t, 3, 2, color.RGBA{R: 40, G: 50, B: 60, A: 255})
	writeProtectedImportFixture(t, server, logicalPath, second)
	secondUpload := StagedUpload{
		Name:           "same.png",
		Path:           logicalPath,
		AnalysisPath:   logicalPath,
		Size:           int64(len(second)),
		TargetID:       "default",
		ConflictPolicy: "rename",
	}
	if _, err := library.importUploadedFiles(context.Background(), []StagedUpload{secondUpload}, []string{"bad tag"}, backgroundUploadImportState{}); err == nil {
		t.Fatal("import with invalid tag error = nil, want validation failure")
	}
	if _, err := os.Stat(logicalPath); err != nil {
		t.Fatalf("failed import did not restore original retry path: %v", err)
	}
	renamedPath := filepath.Join(uploadDir, "same-1.png")
	if _, err := os.Stat(renamedPath); !os.IsNotExist(err) {
		t.Fatalf("failed import left renamed logical path behind: %v", err)
	}

	retryResponse, err := library.importUploadedFiles(context.Background(), []StagedUpload{secondUpload}, []string{"second"}, backgroundUploadImportState{})
	if err != nil {
		t.Fatalf("retry protected upload: %v", err)
	}
	if len(retryResponse.Files) != 1 || retryResponse.Files[0].Status != "imported" || retryResponse.Files[0].Name != "same-1.png" {
		t.Fatalf("retry response = %+v", retryResponse.Files)
	}
	if _, err := os.Stat(logicalPath); !os.IsNotExist(err) {
		t.Fatalf("retry source logical path still exists: %v", err)
	}
	if _, err := os.Stat(renamedPath); !os.IsNotExist(err) {
		t.Fatalf("renamed protected logical path still exists: %v", err)
	}
	if _, err := client.GetFileInfoByPath(renamedPath); err != nil {
		t.Fatalf("retried protected upload was not tracked under renamed path: %v", err)
	}
}

func writeProtectedImportFixture(t *testing.T, server *Server, path string, plaintext []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("create upload directory: %v", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatalf("create protected upload fixture: %v", err)
	}
	if _, err := server.persistUploadedFile(file, bytes.NewReader(plaintext)); err != nil {
		_ = file.Close()
		t.Fatalf("write protected upload fixture: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close protected upload fixture: %v", err)
	}
}
