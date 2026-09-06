package serve

import (
	"bytes"
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

func TestManagedLibraryMovePreservesOriginalAndDerivativeCache(t *testing.T) {
	parent := t.TempDir()
	oldRoot := filepath.Join(parent, "library")
	if err := os.MkdirAll(oldRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	oldDB := filepath.Join(oldRoot, "gooru.db")
	if err := core.Init(oldDB, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(oldDB, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}

	oldUploads := filepath.Join(oldRoot, "uploads")
	oldCache := filepath.Join(oldRoot, "cache")
	cfg := DefaultConfig(oldDB)
	cfg.Auth.Enabled = false
	cfg.Uploads.Enabled = true
	cfg.Uploads.Targets = []UploadTarget{{ID: "default", Name: "Default", Path: oldUploads}}
	cfg.Media.CacheDir = oldCache
	server := NewServerWithLibrary(cfg, NewGooruLibrary(client, false))

	payload := tinyPNG(t, 7, 5, color.RGBA{R: 31, G: 101, B: 211, A: 255})
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadBinaryRequestWithSourceModTime(t, "move-me.png", payload, time.Unix(1_700_000_000, 0)))
	if rec.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", rec.Code, rec.Body.String())
	}
	oldPath := filepath.Join(oldUploads, "move-me.png")
	file, err := client.GetFileInfoByPath(oldPath)
	if err != nil {
		t.Fatalf("lookup upload: %v", err)
	}

	thumb := httptest.NewRecorder()
	server.Handler().ServeHTTP(thumb, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID+"/thumbnail?size=256", nil))
	if thumb.Code != http.StatusOK {
		t.Fatalf("warm thumbnail status=%d body=%s", thumb.Code, thumb.Body.String())
	}
	cacheFiles := derivativeFiles(t, oldCache)
	if len(cacheFiles) == 0 {
		t.Fatal("warm thumbnail did not create a persistent derivative")
	}
	before, err := os.Stat(cacheFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close before move: %v", err)
	}

	newRoot := filepath.Join(parent, "lib1")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range mustGlob(t, oldDB+"*") {
		if err := os.Rename(path, filepath.Join(newRoot, filepath.Base(path))); err != nil {
			t.Fatalf("move database file %s: %v", path, err)
		}
	}
	newUploads := filepath.Join(newRoot, "uploads")
	newCache := filepath.Join(newRoot, "cache")
	if err := os.Rename(oldUploads, newUploads); err != nil {
		t.Fatalf("move uploads: %v", err)
	}
	if err := os.Rename(oldCache, newCache); err != nil {
		t.Fatalf("move cache: %v", err)
	}

	newDB := filepath.Join(newRoot, "gooru.db")
	client2, err := core.New(newDB, false)
	if err != nil {
		t.Fatalf("reopen moved db: %v", err)
	}
	defer client2.Close()
	cfg.Database.Path = newDB
	cfg.Uploads.Targets[0].Path = newUploads
	cfg.Media.CacheDir = newCache
	server2 := NewServerWithLibrary(cfg, NewGooruLibrary(client2, false))

	content := httptest.NewRecorder()
	server2.Handler().ServeHTTP(content, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID+"/content", nil))
	if content.Code != http.StatusOK {
		t.Fatalf("content after move status=%d body=%s", content.Code, content.Body.String())
	}
	if !bytes.Equal(content.Body.Bytes(), payload) {
		t.Fatal("content after move differs from uploaded original")
	}
	relocated, err := client2.GetFileInfoByPublicID(file.PublicID)
	if err != nil {
		t.Fatalf("lookup relocated record: %v", err)
	}
	if relocated.Path != filepath.Join(newUploads, "move-me.png") {
		t.Fatalf("relocated path=%q", relocated.Path)
	}

	thumb2 := httptest.NewRecorder()
	server2.Handler().ServeHTTP(thumb2, httptest.NewRequest(http.MethodGet, "/api/v1/files/"+file.PublicID+"/thumbnail?size=256", nil))
	if thumb2.Code != http.StatusOK {
		t.Fatalf("thumbnail after move status=%d body=%s", thumb2.Code, thumb2.Body.String())
	}
	afterFiles := derivativeFiles(t, newCache)
	if len(afterFiles) != len(cacheFiles) {
		t.Fatalf("derivative count changed after move: before=%d after=%d", len(cacheFiles), len(afterFiles))
	}
	after, err := os.Stat(afterFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("cached derivative was rewritten after move: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func derivativeFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk derivatives: %v", err)
	}
	return out
}

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return paths
}
