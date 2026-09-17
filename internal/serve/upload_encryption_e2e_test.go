package serve

import (
	"bytes"
	"encoding/json"
	"image/color"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/securekey"
	"gooru.local/types"
)

func TestEncryptedUploadImportAndContentGoldenPath(t *testing.T) {
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
	cfg.Encryption.Key = bytes.Repeat([]byte{0x5a}, securekey.Size)
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))
	startTestBackgroundRuntime(t, server, client, "test-protected-upload")

	plaintext := tinyPNG(t, 7, 5, color.RGBA{R: 31, G: 101, B: 211, A: 255})
	sourceModTime := time.Date(2021, time.March, 4, 5, 6, 7, 0, time.UTC)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadBinaryRequestWithSourceModTime(t, "secret.png", plaintext, sourceModTime))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
	}
	waitForTestBackgroundIdle(t, client)

	logicalPath := filepath.Join(uploadDir, "secret.png")
	if _, err := os.Stat(logicalPath); !os.IsNotExist(err) {
		t.Fatalf("protected upload leaked its logical filename on disk: %v", err)
	}

	file, err := client.GetFileInfoByPath(logicalPath)
	if err != nil {
		t.Fatalf("uploaded image was not imported under its logical path: %v", err)
	}
	file, err = client.ResolveManagedStorage(file)
	if err != nil {
		t.Fatalf("resolve protected upload storage: %v", err)
	}
	storedPath := file.StoragePath
	if storedPath == "" || filepath.Ext(storedPath) != "" || filepath.Base(storedPath) == filepath.Base(logicalPath) {
		t.Fatalf("protected upload storage path is not random and extensionless: %q", storedPath)
	}
	stored, err := os.ReadFile(storedPath)
	if err != nil {
		t.Fatalf("read stored upload: %v", err)
	}
	storedInfo, err := os.Stat(storedPath)
	if err != nil {
		t.Fatalf("stat stored upload: %v", err)
	}
	if !storedInfo.ModTime().Equal(sourceModTime) {
		t.Fatalf("encrypted stored modtime=%v want source=%v", storedInfo.ModTime(), sourceModTime)
	}
	if bytes.Equal(stored, plaintext) || bytes.Contains(stored, plaintext) {
		t.Fatal("encrypted upload persisted plaintext bytes")
	}

	opened, err := encryptedfile.Open(storedPath, cfg.Encryption.Key)
	if err != nil {
		t.Fatalf("open encrypted upload: %v", err)
	}
	decrypted, err := io.ReadAll(opened)
	if closeErr := opened.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("read encrypted upload: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted upload differs from request body")
	}

	if file.Size != int64(len(plaintext)) {
		t.Fatalf("registered plaintext size = %d, want %d", file.Size, len(plaintext))
	}
	if file.ModTime != sourceModTime.Unix() {
		t.Fatalf("registered modtime=%d want source=%d", file.ModTime, sourceModTime.Unix())
	}
	metadata, err := client.GetMediaMetadata(file.ID)
	if err != nil {
		t.Fatalf("uploaded image metadata: %v", err)
	}
	if metadata.ImageWidth == nil || *metadata.ImageWidth != 7 || metadata.ImageHeight == nil || *metadata.ImageHeight != 5 {
		t.Fatalf("uploaded image metadata = %+v", metadata)
	}

	content := httptest.NewRecorder()
	server.Handler().ServeHTTP(content, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID+"/content", nil))
	if content.Code != http.StatusOK {
		t.Fatalf("content status = %d: %s", content.Code, content.Body.String())
	}
	if !bytes.Equal(content.Body.Bytes(), plaintext) {
		t.Fatal("served encrypted upload differs from original plaintext")
	}
}

func TestEncryptedUploadHonorsPlaintextSizeLimit(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Encryption.Enabled = true
	cfg.Encryption.Key = bytes.Repeat([]byte{0x2c}, securekey.Size)
	cfg.Uploads.MaxFileSizeBytes = 4
	server := NewServer(cfg)

	dst, err := os.CreateTemp(t.TempDir(), "encrypted-upload-*")
	if err != nil {
		t.Fatal(err)
	}
	defer dst.Close()
	if _, err := server.persistUploadedFile(dst, bytes.NewReader([]byte("12345"))); err != errUploadTooLarge {
		t.Fatalf("persist oversized encrypted upload error = %v, want %v", err, errUploadTooLarge)
	}
}

func TestEncryptedUploadDefaultConflictRenamesLogicalPathAfterHashDedup(t *testing.T) {
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
	cfg.Encryption.Key = bytes.Repeat([]byte{0x33}, securekey.Size)
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))
	startTestBackgroundRuntime(t, server, client, "test-protected-conflict-upload")

	upload := func(data []byte) UploadImportResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, uploadBinaryRequestWithSourceModTime(t, "same.png", data, time.Time{}))
		if rec.Code != http.StatusOK {
			t.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
		}
		var response UploadImportResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if len(response.Files) != 1 {
			t.Fatalf("upload response = %+v", response.Files)
		}
		return response
	}

	firstBytes := tinyPNG(t, 2, 2, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	secondBytes := tinyPNG(t, 3, 2, color.RGBA{R: 40, G: 50, B: 60, A: 255})
	first := upload(firstBytes)
	if first.Files[0].Status != "imported" || first.Files[0].Name != "same.png" {
		t.Fatalf("first upload = %+v", first.Files[0])
	}
	duplicate := upload(firstBytes)
	if duplicate.Files[0].Status != "duplicate_existing" {
		t.Fatalf("exact duplicate = %+v", duplicate.Files[0])
	}
	renamed := upload(secondBytes)
	if renamed.Files[0].Status != "imported" || renamed.Files[0].Name != "same-1.png" {
		t.Fatalf("same-name different-content upload = %+v", renamed.Files[0])
	}

	for _, name := range []string{"same.png", "same-1.png"} {
		logicalPath := filepath.Join(uploadDir, name)
		if _, err := os.Stat(logicalPath); !os.IsNotExist(err) {
			t.Fatalf("protected logical path %q leaked on disk: %v", logicalPath, err)
		}
		file, err := client.GetFileInfoByPath(logicalPath)
		if err != nil {
			t.Fatalf("logical path %q not tracked: %v", logicalPath, err)
		}
		resolved, err := client.ResolveManagedStorage(file)
		if err != nil {
			t.Fatalf("resolve storage for %q: %v", logicalPath, err)
		}
		if resolved.StoragePath == "" || filepath.Ext(resolved.StoragePath) != "" {
			t.Fatalf("protected storage for %q is not opaque: %q", logicalPath, resolved.StoragePath)
		}
	}
}
