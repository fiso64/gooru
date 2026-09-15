package serve

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadImporterCleanupPreservesRacingReplacement(t *testing.T) {
	dir := t.TempDir()
	library := &racingCleanupUploadLibrary{replacement: []byte("racing replacement")}
	server := newUploadTestServer(t, dir, true, library)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"a.txt": "uploaded"}, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !library.sawOwnership {
		t.Fatal("staged upload did not retain ownership evidence during import")
	}
	path := filepath.Join(dir, "a.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("racing replacement was removed: %v", err)
	}
	if got := string(data); got != "racing replacement" {
		t.Fatalf("destination = %q, want racing replacement", got)
	}
}

type racingCleanupUploadLibrary struct {
	recordingUploadLibrary
	replacement  []byte
	sawOwnership bool
}

func (l *racingCleanupUploadLibrary) ImportUploadedFiles(_ context.Context, files []StagedUpload, _ []string) (UploadImportResponse, error) {
	if len(files) != 1 {
		return UploadImportResponse{}, fmt.Errorf("got %d staged uploads, want 1", len(files))
	}
	file := files[0]
	l.sawOwnership = file.OwnershipPath != ""
	if !l.sawOwnership {
		return UploadImportResponse{}, fmt.Errorf("missing staged upload ownership evidence")
	}
	if err := os.Remove(file.Path); err != nil {
		return UploadImportResponse{}, err
	}
	if err := os.WriteFile(file.Path, l.replacement, 0600); err != nil {
		return UploadImportResponse{}, err
	}
	removeRejectedStagedUpload(file)
	return UploadImportResponse{Files: []UploadedFileDTO{{Name: file.Name, Size: file.Size, TargetID: file.TargetID, Status: "duplicate_existing"}}}, nil
}

func TestRemoveStagedUploadPathRemovesOwnedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upload.bin")
	ownershipPath := filepath.Join(dir, ".upload-owned")
	if err := os.WriteFile(path, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, ownershipPath); err != nil {
		t.Fatal(err)
	}

	removeStagedUploadPath(path, ownershipPath)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned path still exists after cleanup: %v", err)
	}
	if data, err := os.ReadFile(ownershipPath); err != nil || string(data) != "owned" {
		t.Fatalf("ownership marker changed: data=%q err=%v", data, err)
	}
}

func TestRemoveStagedUploadPathPreservesRacingReplacementAfterMove(t *testing.T) {
	dir := t.TempDir()
	logicalPath := filepath.Join(dir, "upload.bin")
	ownershipPath := filepath.Join(dir, ".upload-owned")
	storagePath := filepath.Join(dir, "opaque-storage")
	if err := os.WriteFile(logicalPath, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(logicalPath, ownershipPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(logicalPath, storagePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(storagePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storagePath, []byte("racing replacement"), 0600); err != nil {
		t.Fatal(err)
	}

	removeStagedUploadPath(storagePath, ownershipPath)

	data, err := os.ReadFile(storagePath)
	if err != nil {
		t.Fatalf("racing storage replacement was removed: %v", err)
	}
	if got := string(data); got != "racing replacement" {
		t.Fatalf("storage path = %q, want racing replacement", got)
	}
}
