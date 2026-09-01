package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadReplacePreservesOriginalWhenImporterRejectsFile(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	server := newUploadTestServer(t, dir, true, &rejectingUploadLibrary{})
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "replacement"}, []string{"reviewed"}, "", "replace"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected importer response, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(mustReadFile(t, finalPath)); got != "original" {
		t.Fatalf("rejected replacement changed original file: %q", got)
	}
}

type rejectingUploadLibrary struct {
	recordingUploadLibrary
}

func (l *rejectingUploadLibrary) ImportUploadedFiles(_ context.Context, files []StagedUpload, _ []string) (UploadImportResponse, error) {
	response := UploadImportResponse{Files: make([]UploadedFileDTO, 0, len(files))}
	for _, file := range files {
		response.Files = append(response.Files, UploadedFileDTO{
			Name:     file.Name,
			Size:     file.Size,
			TargetID: file.TargetID,
			Status:   "error",
			Error:    "tagging failed",
		})
	}
	return response, nil
}
