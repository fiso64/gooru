package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDurableUploadCanceledRequestDoesNotAttachAfterReadableBodyStages(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadTestStore()
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	req := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)
	req.Header.Set("Prefer", "respond-async")
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusRequestTimeout, "request_canceled")
	if store.attachCalls != 0 {
		t.Fatalf("attach calls = %d, want 0 after request cancellation", store.attachCalls)
	}
	if store.cancelCalls != 1 {
		t.Fatalf("cancel calls = %d, want 1", store.cancelCalls)
	}
	stagingDir, err := durableUploadStagingDir(targetDir, store.operation.ID)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(stagingDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read staging directory after canceled request: %v", err)
	}
	if err == nil && len(entries) != 0 {
		t.Fatalf("staging entries after canceled request = %d, want 0", len(entries))
	}
}
