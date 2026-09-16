package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadConflictPolicyErrorRejectsExistingName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("existing"), 0600); err != nil {
		t.Fatalf("write existing file: %v", err)
	}
	server := newUploadTestServer(t, dir, true, &recordingUploadLibrary{})
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "uploaded"}, nil, "", "error"))
	assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	if got := string(mustReadFile(t, filepath.Join(dir, "a.txt"))); got != "existing" {
		t.Fatalf("existing file was overwritten: %q", got)
	}
}

func TestUploadRejectsRemovedConflictPolicies(t *testing.T) {
	// Removed policy values must stay explicit client errors rather than silently
	// falling back to rename and changing the caller's requested semantics.
	for _, policy := range []string{"skip", "replace"} {
		t.Run(policy, func(t *testing.T) {
			server := newUploadTestServer(t, t.TempDir(), true, &recordingUploadLibrary{})
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "uploaded"}, nil, "", policy))
			assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
		})
	}
}
