package serve

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type orderedUploadPart struct {
	name    string
	content []byte
}

func orderedUploadRequest(t *testing.T, parts []orderedUploadPart, conflictPolicy string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, item := range parts {
		part, err := writer.CreateFormFile("files", item.name)
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := part.Write(item.content); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	if conflictPolicy != "" {
		if err := writer.WriteField("conflict_policy", conflictPolicy); err != nil {
			t.Fatalf("write conflict_policy field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestUploadReplaceRejectsDuplicateDestinationWithinBatch(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "same.txt")
	if err := os.WriteFile(targetPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	library := &recordingUploadLibrary{}
	server := newUploadTestServer(t, dir, true, library)
	winner := []byte("winner payload")
	loser := []byte("loser payload that must not win")
	sibling := []byte("sibling payload")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, orderedUploadRequest(t, []orderedUploadPart{
		{name: "same.txt", content: winner},
		{name: "same.txt", content: loser},
		{name: "sibling.txt", content: sibling},
	}, "replace"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(response.Files) != 3 {
		t.Fatalf("response file count=%d want=3: %+v", len(response.Files), response.Files)
	}
	if response.Files[0].Name != "same.txt" || response.Files[0].Status != "imported" {
		t.Fatalf("first replacement should win, got %+v", response.Files)
	}
	if response.Files[1].Name != "same.txt" || response.Files[1].Status != "error" || response.Files[1].Error != duplicateReplacementDestinationError {
		t.Fatalf("later duplicate should be item-scoped error, got %+v", response.Files)
	}
	if response.Files[2].Name != "sibling.txt" || response.Files[2].Status != "imported" {
		t.Fatalf("unrelated sibling should import, got %+v", response.Files)
	}
	if got := mustReadFile(t, targetPath); !bytes.Equal(got, winner) {
		t.Fatalf("winner bytes=%q want=%q", got, winner)
	}
	if got := mustReadFile(t, filepath.Join(dir, "sibling.txt")); !bytes.Equal(got, sibling) {
		t.Fatalf("sibling bytes=%q want=%q", got, sibling)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("duplicate staging data leaked: %v", entries)
	}
}

func TestGooruUploadReplaceRejectsDuplicateDestinationWithinBatch(t *testing.T) {
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
	if err := os.MkdirAll(uploadDir, 0700); err != nil {
		t.Fatalf("create upload dir: %v", err)
	}
	targetPath := filepath.Join(uploadDir, "same.txt")
	if err := os.WriteFile(targetPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	server := newUploadTestServer(t, uploadDir, true, NewGooruLibrary(client, false))
	startTestBackgroundRuntime(t, server, client, "test-gooru-duplicate-replacement")
	winner := []byte("winner payload")
	loser := []byte("loser payload that has a different size")
	sibling := []byte("unrelated sibling")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, orderedUploadRequest(t, []orderedUploadPart{
		{name: "same.txt", content: winner},
		{name: "same.txt", content: loser},
		{name: "sibling.txt", content: sibling},
	}, "replace"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(response.Files) != 3 {
		t.Fatalf("response file count=%d want=3: %+v", len(response.Files), response.Files)
	}
	if response.Files[0].Status != "imported" {
		t.Fatalf("first replacement should import, got %+v", response.Files)
	}
	if response.Files[1].Status != "error" || response.Files[1].Error != duplicateReplacementDestinationError {
		t.Fatalf("later duplicate should fail independently, got %+v", response.Files)
	}
	if response.Files[2].Status != "imported" {
		t.Fatalf("unrelated sibling should import, got %+v", response.Files)
	}
	if got := mustReadFile(t, targetPath); !bytes.Equal(got, winner) {
		t.Fatalf("winner bytes=%q want=%q", got, winner)
	}
	winnerInfo, err := client.GetFileInfoByPath(targetPath)
	if err != nil {
		t.Fatalf("get winner database row: %v", err)
	}
	if winnerInfo.Size != int64(len(winner)) || winnerInfo.Hash == "" {
		t.Fatalf("winner database metadata does not match first payload: %+v", winnerInfo)
	}
	siblingPath := filepath.Join(uploadDir, "sibling.txt")
	if got := mustReadFile(t, siblingPath); !bytes.Equal(got, sibling) {
		t.Fatalf("sibling bytes=%q want=%q", got, sibling)
	}
	siblingInfo, err := client.GetFileInfoByPath(siblingPath)
	if err != nil {
		t.Fatalf("get sibling database row: %v", err)
	}
	if siblingInfo.Size != int64(len(sibling)) || siblingInfo.Hash == "" {
		t.Fatalf("sibling database metadata mismatch: %+v", siblingInfo)
	}
}
