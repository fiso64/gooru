package serve

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUploadReplaceImporterCleanupPreservesRacingDestination(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	library := &racingReplacementCleanupUploadLibrary{replacement: []byte("racing replacement")}
	server := newUploadTestServer(t, dir, true, library)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "uploaded replacement"}, nil, "", "replace"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected settlement conflict after racing replacement, got %d: %s", rec.Code, rec.Body.String())
	}
	if !library.sawOwnership {
		t.Fatal("replacement did not retain ownership evidence during importer cleanup")
	}
	data, err := os.ReadFile(finalPath)
	if err != nil {
		t.Fatalf("racing destination was removed: %v", err)
	}
	if got := string(data); got != "racing replacement" {
		t.Fatalf("destination = %q, want racing replacement", got)
	}
	stagedPath := strings.TrimSuffix(library.ownershipPath, durableUploadActivatedMarkerSuffix)
	backup, err := os.ReadFile(stagedPath + ".backup")
	if err != nil {
		t.Fatalf("preserved original backup is missing: %v", err)
	}
	if got := string(backup); got != "original" {
		t.Fatalf("preserved original backup = %q, want original", got)
	}
}

type racingReplacementCleanupUploadLibrary struct {
	recordingUploadLibrary
	replacement   []byte
	ownershipPath string
	sawOwnership  bool
}

func (l *racingReplacementCleanupUploadLibrary) ImportUploadedFiles(_ context.Context, files []StagedUpload, _ []string) (UploadImportResponse, error) {
	if len(files) != 1 {
		return UploadImportResponse{}, fmt.Errorf("got %d staged uploads, want 1", len(files))
	}
	file := files[0]
	l.ownershipPath = file.OwnershipPath
	if file.OwnershipPath != "" {
		ownedInfo, ownedErr := os.Stat(file.OwnershipPath)
		currentInfo, currentErr := os.Stat(file.Path)
		l.sawOwnership = ownedErr == nil && currentErr == nil && os.SameFile(ownedInfo, currentInfo)
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
