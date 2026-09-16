package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadConflictPolicyErrorRejectsWholeBatchOnExistingName(t *testing.T) {
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

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("valid sibling should be rolled back when the batch is rejected: %v", err)
	}
	if got := string(mustReadFile(t, filepath.Join(dir, "b.txt"))); got != "existing" {
		t.Fatalf("existing file was changed: %q", got)
	}
}
