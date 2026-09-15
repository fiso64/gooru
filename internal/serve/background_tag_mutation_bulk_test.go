package serve

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "gosqlite.org"
)

func TestTagMutationIntegrationUpdatesMultipleExplicitFiles(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()

	runtime, err := server.NewBackgroundRuntime(client, "bulk-tag-mutation-test")
	if err != nil {
		t.Fatalf("create background runtime: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("stop background runtime: %v", err)
		}
	}()

	page := listTestFiles(t, server, "kind:image", 2)
	if len(page.Files) != 2 {
		t.Fatalf("expected two image files, got %d", len(page.Files))
	}
	body, err := json.Marshal(map[string]any{
		"file_ids": []string{page.Files[1].ID, page.Files[0].ID},
		"tags":     []string{"bulk-reviewed"},
	})
	if err != nil {
		t.Fatal(err)
	}
	mutateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(mutateRec, authedJSONRequest(http.MethodPost, "/api/v1/files/tags", string(body)))
	if mutateRec.Code != http.StatusOK {
		t.Fatalf("expected mutation 200, got %d: %s", mutateRec.Code, mutateRec.Body.String())
	}

	for _, selected := range page.Files {
		detailRec := httptest.NewRecorder()
		server.Handler().ServeHTTP(detailRec, authedRequest(http.MethodGet, "/api/v1/files/"+selected.ID))
		if detailRec.Code != http.StatusOK {
			t.Fatalf("expected detail 200 for %s, got %d: %s", selected.ID, detailRec.Code, detailRec.Body.String())
		}
		var detail FileDTO
		if err := json.Unmarshal(detailRec.Body.Bytes(), &detail); err != nil {
			t.Fatalf("decode detail response: %v", err)
		}
		if !containsString(detail.Tags, "bulk-reviewed") {
			t.Fatalf("expected updated tags for %s, got %+v", selected.ID, detail.Tags)
		}
	}
}

func TestGetFilesByPublicIDsReturnsManagedStorageMapping(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gooru.db")
	server, client := newTestBrowseServerAt(t, dir, dbPath)
	defer client.Close()

	files, err := client.GetAllFilesInfo()
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("expected tracked files")
	}
	selected := files[0]
	physicalPath := filepath.Join(dir, "managed", "opaque-container")

	library, ok := server.library.(*GooruLibrary)
	if !ok {
		t.Fatalf("library type = %T, want *GooruLibrary", server.library)
	}
	publicID := library.PublicFileID(selected)
	if publicID == "" {
		t.Fatal("expected stable public file ID")
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO managed_storage_locations (location_id, physical_path) VALUES (?, ?)`, selected.ID, physicalPath); err != nil {
		t.Fatalf("insert managed storage mapping: %v", err)
	}

	library.managedRoots = []string{dir}
	got, err := library.GetFilesByPublicIDs(context.Background(), []string{publicID})
	if err != nil {
		t.Fatalf("GetFilesByPublicIDs: %v", err)
	}
	if len(got) != 1 || got[0].StoragePath != physicalPath {
		t.Fatalf("managed file = %+v, want storage path %q", got, physicalPath)
	}
}
