package serve

import (
	"context"
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

func TestUploadReplacePreservesOriginalWhenImporterFails(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	library := &failingUploadLibrary{err: errors.New("import failed")}
	server := newUploadTestServer(t, dir, true, library)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "replacement"}, nil, "", "replace"))

	if rec.Code == http.StatusOK || rec.Code == http.StatusAccepted {
		t.Fatalf("expected failed import, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := string(mustReadFile(t, finalPath)); got != "original" {
		t.Fatalf("failed import changed original file: %q", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read upload dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "a.txt" {
		t.Fatalf("failed replacement left staged files behind: %+v", entries)
	}
}

func TestGooruUploadReplaceDuplicateExistingPreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	trackedDir := filepath.Join(dir, "tracked")
	if err := os.MkdirAll(trackedDir, 0700); err != nil {
		t.Fatalf("create tracked dir: %v", err)
	}
	trackedPath := filepath.Join(trackedDir, "tracked.txt")
	if err := os.WriteFile(trackedPath, []byte("duplicate content"), 0600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	if _, err := client.TagFiles([]string{trackedPath}, []string{"state:tracked"}, nil, false); err != nil {
		t.Fatalf("track duplicate content: %v", err)
	}

	uploadDir := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadDir, 0700); err != nil {
		t.Fatalf("create upload dir: %v", err)
	}
	finalPath := filepath.Join(uploadDir, "a.txt")
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	server := newUploadTestServer(t, uploadDir, true, NewGooruLibrary(client, false))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{"a.txt": "duplicate content"}, nil, "", "replace"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Files) != 1 || response.Files[0].Status != "duplicate_existing" {
		t.Fatalf("expected duplicate_existing, got %+v", response.Files)
	}
	if got := string(mustReadFile(t, finalPath)); got != "original" {
		t.Fatalf("duplicate replacement changed original file: %q", got)
	}
}

func TestGooruUploadReplaceDuplicateInBatchPreservesRejectedOriginal(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	if err := core.Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init db: %v", err)
	}
	client, err := core.New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	defer client.Close()

	uploadDir := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadDir, 0700); err != nil {
		t.Fatalf("create upload dir: %v", err)
	}
	originals := map[string]string{"a.txt": "original-a", "b.txt": "original-b"}
	for name, content := range originals {
		if err := os.WriteFile(filepath.Join(uploadDir, name), []byte(content), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	server := newUploadTestServer(t, uploadDir, true, NewGooruLibrary(client, false))
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequestWithConflict(t, map[string]string{
		"a.txt": "same new content",
		"b.txt": "same new content",
	}, nil, "", "replace"))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	var duplicateName string
	for _, file := range response.Files {
		if file.Status == "duplicate_in_batch" {
			duplicateName = file.Name
			break
		}
	}
	if duplicateName == "" {
		t.Fatalf("expected a duplicate_in_batch result, got %+v", response.Files)
	}
	if got := string(mustReadFile(t, filepath.Join(uploadDir, duplicateName))); got != originals[duplicateName] {
		t.Fatalf("duplicate-in-batch replacement changed %s: %q", duplicateName, got)
	}
}

func TestUploadReplacementRollbackRestoresOriginal(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "a.txt")
	stagedPath := filepath.Join(dir, ".a.txt.tmp")
	if err := os.WriteFile(finalPath, []byte("original"), 0600); err != nil {
		t.Fatalf("write original: %v", err)
	}
	if err := os.WriteFile(stagedPath, []byte("replacement"), 0600); err != nil {
		t.Fatalf("write staged replacement: %v", err)
	}

	replacement, err := activateReplacement(stagedPath, finalPath)
	if err != nil {
		t.Fatalf("activate replacement: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "replacement" {
		t.Fatalf("replacement was not activated: %q", got)
	}
	if err := rollbackReplacement(replacement); err != nil {
		t.Fatalf("rollback replacement: %v", err)
	}
	if got := string(mustReadFile(t, finalPath)); got != "original" {
		t.Fatalf("rollback did not restore original: %q", got)
	}
}

type failingUploadLibrary struct {
	recordingUploadLibrary
	err error
}

func (l *failingUploadLibrary) ImportUploadedFiles(_ context.Context, _ []StagedUpload, _ []string) (UploadImportResponse, error) {
	return UploadImportResponse{}, l.err
}
