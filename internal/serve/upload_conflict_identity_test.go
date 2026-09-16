package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestUploadSameNameSameContentRemainsHashDuplicate(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	uploadDir := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadDir, 0o700); err != nil {
		t.Fatalf("create upload dir: %v", err)
	}
	const contents = "same content"
	existingPath := writeTestFile(t, uploadDir, "same.txt", contents)
	if _, err := client.TagFiles([]string{existingPath}, []string{"state:existing"}, nil, false); err != nil {
		t.Fatalf("track existing file: %v", err)
	}
	existingInfo, err := client.GetFileInfoByPath(existingPath)
	if err != nil {
		t.Fatalf("get existing file: %v", err)
	}
	existingID := client.PublicFileID(existingInfo.ID)

	server := newUploadTestServer(t, uploadDir, true, NewGooruLibrary(client, false))
	startTestBackgroundRuntime(t, server, client, "test-same-name-same-content")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"same.txt": contents}, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(response.Files) != 1 || response.Files[0].Status != "duplicate_existing" {
		t.Fatalf("same-name same-content response = %+v, want duplicate_existing", response.Files)
	}
	if response.Files[0].ID != existingID {
		t.Fatalf("duplicate id = %q, want existing id %q", response.Files[0].ID, existingID)
	}
	if got := string(mustReadFile(t, existingPath)); got != contents {
		t.Fatalf("existing file changed to %q", got)
	}
	if _, err := os.Stat(filepath.Join(uploadDir, "same-1.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("renamed duplicate should be discarded, stat error = %v", err)
	}
}
