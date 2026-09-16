package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadConflictPolicyErrorIsolatesExistingNameWithinBatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("existing"), 0600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}
	server := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{
		"a.txt": "valid",
		"b.txt": "replacement",
	}, nil, "", "error"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	byName := make(map[string]UploadedFileDTO, len(response.Files))
	for _, file := range response.Files {
		byName[file.Name] = file
	}
	if byName["a.txt"].Status != "imported" {
		t.Fatalf("valid sibling should import, got %+v", byName)
	}
	if byName["b.txt"].Status != "error" || byName["b.txt"].Error != errUploadConflict.Error() {
		t.Fatalf("conflicting member should fail independently, got %+v", byName)
	}
	if got := string(mustReadFile(t, filepath.Join(dir, "a.txt"))); got != "valid" {
		t.Fatalf("valid sibling was not retained: %q", got)
	}
	if got := string(mustReadFile(t, filepath.Join(dir, "b.txt"))); got != "existing" {
		t.Fatalf("existing file was changed: %q", got)
	}
}
