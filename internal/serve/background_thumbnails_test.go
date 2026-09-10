package serve

import (
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestBackgroundThumbnailTaskUsesBrowsingDerivativeIdentity(t *testing.T) {
	cfg := DefaultConfig("")
	cfg.Media.ThumbnailSizes = []int{320, 640}
	cfg.Media.ThumbnailFormat = "jpeg"
	media := NewMediaService(cfg)
	location := types.LocationInfo{Path: "/library/photo.jpg", Hash: "content-hash"}

	tasks := media.backgroundTaskRequests(location)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	_, _, expected, ok := media.browsingThumbnailSpec(types.FileInfo{Path: location.Path, Hash: location.Hash})
	if !ok {
		t.Fatal("browsing thumbnail unexpectedly unsupported")
	}
	task := tasks[0]
	if task.InputKey != expected || task.DedupeKey != "thumbnail:"+expected || task.SubjectID != location.Hash || task.ResourceClass != backgroundThumbnailResourceClass {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestBackgroundThumbnailHandlerUsesCurrentContentLocation(t *testing.T) {
	cfg := DefaultConfig("")
	cfg.Media.ThumbnailSizes = nil
	server := NewServer(cfg)
	server.backgroundContent = fakeContentHashLibrary{file: types.FileInfo{Path: "/moved/photo.jpg", Hash: "hash"}}
	task := core.BackgroundTask{Kind: backgroundThumbnailTaskKind, SubjectKind: "content", SubjectID: "hash"}
	if err := server.backgroundThumbnailHandler(context.Background(), task); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func TestUploadDurablySchedulesAndGeneratesBrowsingThumbnail(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	writeInitializedTestDB(t, dbPath, types.StrategyFull)
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	uploadDir := filepath.Join(dir, "uploads")
	cacheDir := filepath.Join(dir, "cache")
	cfg := DefaultConfig(filepath.Join(dir, "serve.db"))
	cfg.Auth.Enabled = false
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: uploadDir}}
	cfg.Media.CacheDir = cacheDir
	cfg.Media.ThumbnailSizes = []int{32}
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))

	rec := httptest.NewRecorder()
	req := uploadBinaryRequest(t, map[string][]byte{
		"photo.png": tinyPNG(t, 64, 48, color.White),
	}, nil)
	req.Header.Set("Prefer", "respond-async")
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("async upload status = %d: %s", rec.Code, rec.Body.String())
	}
	var operation BackgroundOperationDTO
	if err := json.NewDecoder(rec.Body).Decode(&operation); err != nil {
		t.Fatalf("decode upload operation: %v", err)
	}
	if operation.ID == "" || operation.Kind != "upload_import" {
		t.Fatalf("unexpected upload operation: %+v", operation)
	}

	storedPath := filepath.Join(uploadDir, "photo.png")
	if _, err := client.GetFileInfoByPath(storedPath); err == nil {
		t.Fatal("upload was imported before the durable worker started")
	}

	startTestBackgroundRuntime(t, server, client, "test-eager-thumbnail")

	deadline := time.Now().Add(5 * time.Second)
	var file types.FileInfo
	for {
		file, err = client.GetFileInfoByPath(storedPath)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("durable upload task did not import %s: %v", storedPath, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, _, relativePath, ok := server.media.browsingThumbnailSpec(file)
	if !ok {
		t.Fatal("uploaded image unexpectedly has no browsing thumbnail spec")
	}
	cachePath, err := derivativePath(cacheDir, relativePath)
	if err != nil {
		t.Fatalf("derivative path: %v", err)
	}

	for {
		if _, err := os.Stat(cachePath); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat generated thumbnail: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("durable thumbnail task did not populate cache at %s", cachePath)
		}
		time.Sleep(20 * time.Millisecond)
	}

	state, found, err := server.backgroundOperations.GetBackgroundOperation(operation.ID)
	if err != nil || !found {
		t.Fatalf("load upload operation: found=%v err=%v", found, err)
	}
	if state.Status != core.BackgroundWorkCompleted {
		t.Fatalf("upload operation status = %s, want completed", state.Status)
	}

	thumbnail := httptest.NewRecorder()
	server.Handler().ServeHTTP(thumbnail, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID+"/thumbnail?size=32", nil))
	if thumbnail.Code != http.StatusOK {
		t.Fatalf("thumbnail status = %d: %s", thumbnail.Code, thumbnail.Body.String())
	}
	if got := thumbnail.Header().Get("X-Gooru-Cache"); got != "hit" {
		t.Fatalf("thumbnail cache status = %q, want hit", got)
	}
	if thumbnail.Body.Len() == 0 {
		t.Fatal("generated thumbnail response is empty")
	}
}

type fakeContentHashLibrary struct{ file types.FileInfo }

func (f fakeContentHashLibrary) GetFileByContentHash(context.Context, string) (types.FileInfo, error) {
	return f.file, nil
}
