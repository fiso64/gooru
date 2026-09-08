package serve

import (
	"context"
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
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
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
	server.Handler().ServeHTTP(rec, uploadBinaryRequest(t, map[string][]byte{
		"photo.png": tinyPNG(t, 64, 48, color.White),
	}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d: %s", rec.Code, rec.Body.String())
	}

	storedPath := filepath.Join(uploadDir, "photo.png")
	file, err := client.GetFileInfoByPath(storedPath)
	if err != nil {
		t.Fatalf("uploaded image was not imported: %v", err)
	}
	_, _, relativePath, ok := server.media.browsingThumbnailSpec(file)
	if !ok {
		t.Fatal("uploaded image unexpectedly has no browsing thumbnail spec")
	}
	cachePath, err := derivativePath(cacheDir, relativePath)
	if err != nil {
		t.Fatalf("derivative path: %v", err)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("thumbnail should not be generated inline with upload; stat err = %v", err)
	}

	runtime, err := server.NewBackgroundRuntime(client, "test-eager-thumbnail")
	if err != nil {
		t.Fatalf("background runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case runErr := <-done:
			if runErr != nil && !errors.Is(runErr, context.Canceled) {
				t.Errorf("background runtime shutdown: %v", runErr)
			}
		case <-time.After(2 * time.Second):
			t.Error("background runtime did not stop after cancellation")
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
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
