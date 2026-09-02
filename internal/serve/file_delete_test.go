package serve

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestDeleteModeRemovesManagedUploadFileAndLocation(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	page := listTestFiles(t, server, "kind:image", 1)
	file, err := server.getFileByPublicID(context.Background(), page.Files[0].ID)
	if err != nil {
		t.Fatalf("resolve file: %v", err)
	}
	server.cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: filepath.Dir(file.Path)}}

	refreshed := listTestFiles(t, server, "kind:image", 1)
	if !refreshed.Files[0].CanDelete {
		t.Fatal("expected file under configured upload target to advertise physical deletion")
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+page.Files[0].ID, bytes.NewBufferString(`{"mode":"delete"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected delete 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(file.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected managed file to be removed from disk, stat err=%v", err)
	}
	missing := httptest.NewRecorder()
	server.Handler().ServeHTTP(missing, authedRequest(http.MethodGet, "/api/v1/files/"+page.Files[0].ID))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("expected deleted location 404, got %d: %s", missing.Code, missing.Body.String())
	}
}

func TestDeleteModeRejectsFileOutsideUploadTargets(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	page := listTestFiles(t, server, "kind:image", 1)
	file, err := server.getFileByPublicID(context.Background(), page.Files[0].ID)
	if err != nil {
		t.Fatalf("resolve file: %v", err)
	}
	server.cfg.Uploads.Targets = []UploadTarget{{ID: "other", Name: "Other", Path: t.TempDir()}}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files/"+page.Files[0].ID, bytes.NewBufferString(`{"mode":"delete"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	assertAPIError(t, rec, http.StatusConflict, "file_not_managed")
	if _, err := os.Stat(file.Path); err != nil {
		t.Fatalf("unmanaged file should remain on disk: %v", err)
	}
	detail := httptest.NewRecorder()
	server.Handler().ServeHTTP(detail, authedRequest(http.MethodGet, "/api/v1/files/"+page.Files[0].ID))
	if detail.Code != http.StatusOK {
		t.Fatalf("unmanaged file should remain tracked, got %d: %s", detail.Code, detail.Body.String())
	}
}

func TestManagedDeleteRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.jpg")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	cfg := DefaultConfig(filepath.Join(root, "gooru.db"))
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: root}}
	server := NewServerWithLibrary(cfg, emptyLibrary{})
	if server.canDeleteFilePath(filepath.Join(link, "outside.jpg")) {
		t.Fatal("symlinked path escaping the configured target must not be physically deletable")
	}
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("outside file unexpectedly changed: %v", err)
	}
}

func TestManagedDeleteRollsBackFileWhenLibraryMutationFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kept.jpg")
	if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	library := failingPhysicalDeleteLibrary{file: types.FileInfo{ID: 1, PublicID: "public", Path: path}}
	cfg := DefaultConfig(filepath.Join(dir, "gooru.db"))
	cfg.Auth.Enabled = false
	cfg.Uploads.Targets = []UploadTarget{{ID: "managed", Name: "Managed", Path: dir}}
	server := NewServerWithLibrary(cfg, library)

	if _, err := server.deleteManagedFile(context.Background(), "public"); err == nil {
		t.Fatal("expected library deletion failure")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "keep me" {
		t.Fatalf("file was not restored after library failure: body=%q err=%v", body, err)
	}
}

type failingPhysicalDeleteLibrary struct {
	file types.FileInfo
}

func (l failingPhysicalDeleteLibrary) ListFiles(context.Context, string) ([]types.FileInfo, error) {
	return []types.FileInfo{l.file}, nil
}
func (l failingPhysicalDeleteLibrary) GetFile(context.Context, int64) (types.FileInfo, error) {
	return l.file, nil
}
func (l failingPhysicalDeleteLibrary) ListTags(context.Context, bool, int) ([]TagDTO, error) {
	return nil, nil
}
func (l failingPhysicalDeleteLibrary) PublicFileID(types.FileInfo) string { return "public" }
func (l failingPhysicalDeleteLibrary) GetFileByPublicID(context.Context, string) (types.FileInfo, error) {
	return l.file, nil
}
func (l failingPhysicalDeleteLibrary) DeleteFileByPublicID(context.Context, string) (bool, error) {
	return false, errors.New("database write failed")
}
