package serve

import (
	"bytes"
	"context"
	"encoding/json"
	"image/color"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestCBZUploadImportAndOpenGoldenPath(t *testing.T) {
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
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))
	startTestBackgroundRuntime(t, server, client, "test-cbz-upload")

	page := tinyPNG(t, 3, 2, color.White)
	comicPath := writeComic(t, map[string][]byte{"pages/1.png": page})
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadBinaryRequest(t, map[string][]byte{"book.cbz": mustReadFile(t, comicPath)}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
	}
	waitForTestBackgroundIdle(t, client)

	storedPath := filepath.Join(uploadDir, "book.cbz")
	file, err := client.GetFileInfoByPath(storedPath)
	if err != nil {
		t.Fatalf("uploaded comic was not imported: %v", err)
	}
	if file.PublicID == "" {
		t.Fatal("uploaded comic has no public id")
	}
	if file.Metadata == nil || file.Metadata.PageCount == nil || *file.Metadata.PageCount != 1 {
		t.Fatalf("uploaded comic page count = %+v, want 1", file.Metadata)
	}
	fileResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(fileResponse, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID, nil))
	if fileResponse.Code != http.StatusOK {
		t.Fatalf("file metadata status = %d: %s", fileResponse.Code, fileResponse.Body.String())
	}
	var fileDTO FileDTO
	if err := json.Unmarshal(fileResponse.Body.Bytes(), &fileDTO); err != nil {
		t.Fatalf("decode file metadata: %v", err)
	}
	if fileDTO.Metadata.PageCount == nil || *fileDTO.Metadata.PageCount != 1 {
		t.Fatalf("file API page count = %+v, want 1", fileDTO.Metadata.PageCount)
	}

	manifest := httptest.NewRecorder()
	server.Handler().ServeHTTP(manifest, httptest.NewRequest(http.MethodGet, "/api/v1/comics/"+file.PublicID, nil))
	if manifest.Code != http.StatusOK {
		t.Fatalf("comic manifest status = %d: %s", manifest.Code, manifest.Body.String())
	}

	pageResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(pageResponse, httptest.NewRequest(http.MethodGet, "/api/v1/comics/"+file.PublicID+"/0", nil))
	if pageResponse.Code != http.StatusOK {
		t.Fatalf("comic page status = %d: %s", pageResponse.Code, pageResponse.Body.String())
	}
	if !bytes.Equal(pageResponse.Body.Bytes(), page) {
		t.Fatal("comic page bytes differ from uploaded archive page")
	}
}

func TestCBZUploadCanProgressLongerThanServerIOTimeouts(t *testing.T) {
	uploadDir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, uploadDir, true, library)
	server.cfg.Server.ReadTimeout = 40 * time.Millisecond
	server.cfg.Server.WriteTimeout = 40 * time.Millisecond
	server.cfg.Server.MaxRequestBodyBytes = 2 << 20

	page := tinyPNG(t, 3, 2, color.White)
	comicPath := writeComic(t, map[string][]byte{
		"pages/1.png": page,
		"padding.bin": deterministicNoise(128 << 10),
	})
	prepared := uploadBinaryRequest(t, map[string][]byte{"book.cbz": mustReadFile(t, comicPath)}, nil)
	body, err := io.ReadAll(prepared.Body)
	if err != nil {
		t.Fatalf("read prepared multipart body: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	httpServer := server.HTTPServer()
	serveDone := make(chan error, 1)
	go func() { serveDone <- httpServer.Serve(listener) }()
	defer func() {
		_ = httpServer.Shutdown(context.Background())
		<-serveDone
	}()

	request, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+"/api/v1/uploads", &pacedReader{
		reader: bytes.NewReader(body),
		chunk:  4096,
		delay:  5 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	request.Header.Set("Content-Type", prepared.Header.Get("Content-Type"))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("upload request failed after making steady progress: %v", err)
	}
	defer response.Body.Close()
	responseBody, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d: %s", response.StatusCode, responseBody)
	}
	if len(library.files) != 1 || filepath.Base(library.files[0].Path) != "book.cbz" {
		t.Fatalf("comic upload did not reach importer: %+v", library.files)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, "book.cbz")); err != nil {
		t.Fatalf("comic upload was not stored: %v", err)
	}
}

func TestCBZUploadIsNotCappedByGenericAPIRequestLimit(t *testing.T) {
	uploadDir := t.TempDir()
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, uploadDir, true, library)
	server.cfg.Server.MaxRequestBodyBytes = 512

	page := tinyPNG(t, 3, 2, color.White)
	comicPath := writeComic(t, map[string][]byte{
		"pages/1.png": page,
		"padding.bin": deterministicNoise(8 << 10),
	})
	comic := mustReadFile(t, comicPath)
	if len(comic) <= int(server.cfg.Server.MaxRequestBodyBytes) {
		t.Fatalf("test archive is only %d bytes; need more than generic limit", len(comic))
	}

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadBinaryRequest(t, map[string][]byte{"book.cbz": comic}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(library.files) != 1 || filepath.Base(library.files[0].Path) != "book.cbz" {
		t.Fatalf("comic upload did not reach importer: %+v", library.files)
	}
}

type pacedReader struct {
	reader *bytes.Reader
	chunk  int
	delay  time.Duration
}

func (r *pacedReader) Read(p []byte) (int, error) {
	if len(p) > r.chunk {
		p = p[:r.chunk]
	}
	time.Sleep(r.delay)
	return r.reader.Read(p)
}

func deterministicNoise(size int) []byte {
	out := make([]byte, size)
	var state uint32 = 0x12345678
	for i := range out {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		out[i] = byte(state)
	}
	return out
}
