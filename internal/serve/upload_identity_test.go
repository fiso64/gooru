package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

func TestGooruUploadImportReturnsStableFileIdentities(t *testing.T) {
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

	existingPath := writeTestFile(t, dir, "existing-source.txt", "already tracked")
	if _, err := client.TagFiles([]string{existingPath}, []string{"state:existing"}, nil, false); err != nil {
		t.Fatalf("tag existing file: %v", err)
	}
	existingInfo, err := client.GetFileInfoByPath(existingPath)
	if err != nil {
		t.Fatalf("get existing file: %v", err)
	}
	existingID := client.PublicFileID(existingInfo.ID)
	if existingID == "" {
		t.Fatal("existing tracked file is missing public id")
	}

	uploadDir := filepath.Join(dir, "uploads")
	server := newUploadTestServer(t, uploadDir, true, NewGooruLibrary(client, false))
	startTestBackgroundRuntime(t, server, client, "test-gooru-upload-identities")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{
		"duplicate-existing.txt": "already tracked",
		"fresh-a.txt":            "new content",
		"fresh-b.txt":            "new content",
	}, []string{"uploaded"}))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(response.Files) != 3 {
		t.Fatalf("upload response files = %d, want 3: %+v", len(response.Files), response.Files)
	}
	if response.AffectedCount != 2 {
		t.Fatalf("upload affected_count = %d, want 2 combined duplicate/new tag associations", response.AffectedCount)
	}

	seenImported := false
	seenDuplicate := false
	seenBatchDuplicate := false
	importedID := ""
	batchDuplicateID := ""
	for _, file := range response.Files {
		switch file.Status {
		case "duplicate_existing":
			seenDuplicate = true
			if file.ID != existingID {
				t.Fatalf("duplicate id = %q, want existing public id %q", file.ID, existingID)
			}
		case "imported":
			seenImported = true
			importedID = file.ID
			if file.ID == "" {
				t.Fatalf("imported file %q is missing public id", file.Name)
			}
			info, err := client.GetFileInfoByPublicID(file.ID)
			if err != nil {
				t.Fatalf("resolve imported public id %q: %v", file.ID, err)
			}
			if filepath.Base(info.Path) != file.Name {
				t.Fatalf("imported public id resolves to %q, want basename %q", info.Path, file.Name)
			}
		case "duplicate_in_batch":
			seenBatchDuplicate = true
			batchDuplicateID = file.ID
			if file.ID == "" {
				t.Fatalf("same-batch duplicate %q is missing canonical public id", file.Name)
			}
		}
	}
	if !seenImported || !seenDuplicate || !seenBatchDuplicate {
		t.Fatalf("expected imported, duplicate_existing, and duplicate_in_batch identities, got %+v", response.Files)
	}
	if batchDuplicateID != importedID {
		t.Fatalf("same-batch duplicate id = %q, want canonical imported id %q", batchDuplicateID, importedID)
	}

	updatedExisting, err := client.GetFileInfoByPath(existingPath)
	if err != nil {
		t.Fatalf("reload existing duplicate: %v", err)
	}
	tagSet := make(map[string]bool, len(updatedExisting.Tags))
	for _, tag := range updatedExisting.Tags {
		tagSet[tag] = true
	}
	if !tagSet["state:existing"] || !tagSet["uploaded"] {
		t.Fatalf("existing duplicate tags = %v, want preserved state:existing plus uploaded", updatedExisting.Tags)
	}
}

func TestGooruUploadImportRestoresKnownContentWithoutTrackedLocation(t *testing.T) {
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

	const contents = "known content without a tracked location"
	sourcePath := writeTestFile(t, dir, "old-source.txt", contents)
	if _, err := client.TagFiles([]string{sourcePath}, []string{"state:retained"}, nil, false); err != nil {
		t.Fatalf("tag source file: %v", err)
	}
	original, err := client.GetFileInfoByPath(sourcePath)
	if err != nil {
		t.Fatalf("get source file: %v", err)
	}
	deleted, err := client.DeleteLocationByID(original.ID)
	if err != nil || !deleted {
		t.Fatalf("untrack source location = %v, %v; want true, nil", deleted, err)
	}
	if _, err := client.GetFileInfoByContentHash(original.Hash); !errors.Is(err, core.ErrContentNotTracked) {
		t.Fatalf("content location after untrack = %v, want ErrContentNotTracked", err)
	}

	uploadDir := filepath.Join(dir, "uploads")
	server := newUploadTestServer(t, uploadDir, true, NewGooruLibrary(client, false))
	startTestBackgroundRuntime(t, server, client, "test-gooru-upload-untracked-content")
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, uploadRequest(t, map[string]string{"restored.txt": contents}, []string{"uploaded"}))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response UploadImportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode upload response: %v", err)
	}
	if len(response.Files) != 1 || response.Files[0].Status != "imported" || response.Files[0].ID == "" {
		t.Fatalf("restored upload response = %+v, want one imported file with public id", response.Files)
	}
	restored, err := client.GetFileInfoByPublicID(response.Files[0].ID)
	if err != nil {
		t.Fatalf("resolve restored upload: %v", err)
	}
	if filepath.Dir(restored.Path) != uploadDir {
		t.Fatalf("restored path = %q, want upload directory %q", restored.Path, uploadDir)
	}
	tagSet := make(map[string]bool, len(restored.Tags))
	for _, tag := range restored.Tags {
		tagSet[tag] = true
	}
	if !tagSet["uploaded"] {
		t.Fatalf("restored tags = %v, want submitted upload tag", restored.Tags)
	}
}

type uploadIdentityReplayImporter struct {
	recordingUploadImporter
	enrichCalls int
}

func (i *uploadIdentityReplayImporter) populateUploadFileIDs(response *UploadImportResponse, _ []StagedUpload) error {
	i.enrichCalls++
	for index := range response.Files {
		response.Files[index].ID = "recovered-public-id"
	}
	return nil
}

func TestRunBackgroundUploadTaskEnrichesImportedCheckpointBeforePublishingResult(t *testing.T) {
	operationID := "operation-identity-replay"
	response := UploadImportResponse{Files: []UploadedFileDTO{{Name: "already-rejected.jpg", Size: 12, TargetID: "default", Status: "error", Error: "rejected"}}}
	importer := &uploadIdentityReplayImporter{}
	store := &recordingUploadWorkerStore{
		checkpoint:      backgroundUploadImportedCheckpoint(nil, response),
		found:           true,
		operationStatus: core.BackgroundWorkRunning,
	}

	if err := runBackgroundUploadTask(context.Background(), importer, store, backgroundUploadWorkerTestTask(t, operationID)); err != nil {
		t.Fatalf("replay upload task: %v", err)
	}
	if importer.calls != 0 {
		t.Fatalf("import calls = %d, want 0", importer.calls)
	}
	if importer.enrichCalls != 1 {
		t.Fatalf("identity enrichment calls = %d, want 1", importer.enrichCalls)
	}
	if got := store.result.Files[0].ID; got != "recovered-public-id" {
		t.Fatalf("published replay id = %q, want recovered-public-id", got)
	}
	if response.Files[0].ID != "" {
		t.Fatalf("checkpoint response was mutated in place: %+v", response.Files[0])
	}
}
