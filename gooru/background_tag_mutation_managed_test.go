package gooru

import (
	"os"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

func TestBackgroundTagMutationManagedFileUsesStoragePath(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	logicalPath := filepath.Join(dir, "logical", "photo.jpg")
	storagePath := writeBackgroundTagMutationTestFile(t, dir, "opaque-storage.bin", "managed plaintext")

	metadata, err := client.hasher.FileMetadata(storagePath)
	if err != nil {
		t.Fatalf("read managed metadata: %v", err)
	}
	hash, err := client.hasher.HashFile(storagePath)
	if err != nil {
		t.Fatalf("hash managed file: %v", err)
	}
	if _, err := client.TagKnownFiles([]types.LocationInfo{{
		Path:        logicalPath,
		StoragePath: storagePath,
		Hash:        hash,
		Size:        metadata.Size,
		ModTime:     metadata.ModTime.Unix(),
		Extension:   filepath.Ext(logicalPath),
	}}, []string{"initial"}, nil); err != nil {
		t.Fatalf("register managed file: %v", err)
	}
	if _, err := os.Stat(logicalPath); !os.IsNotExist(err) {
		t.Fatalf("logical path should remain absent, stat err=%v", err)
	}

	files, err := client.GetAllFilesInfo()
	if err != nil || len(files) != 1 {
		t.Fatalf("list managed file: files=%v err=%v", files, err)
	}
	if files[0].StoragePath != storagePath {
		t.Fatalf("storage path = %q, want %q", files[0].StoragePath, storagePath)
	}
	publicID := client.PublicFileID(files[0].ID)
	if publicID == "" {
		t.Fatal("missing public file id")
	}
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:       "add",
		Selector:       map[string][]string{"file_ids": {publicID}},
		Tags:           []string{"reviewed"},
		FileIDs:        []string{publicID},
		FileIDSelector: true,
		MaxPending:     8,
	})
	if err != nil {
		t.Fatalf("create background mutation: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationFiles(operation.ID, files); err != nil {
		t.Fatalf("execute managed-file mutation: %v", err)
	}

	got, err := client.GetFileInfoByPublicID(publicID)
	if err != nil {
		t.Fatalf("reload managed file: %v", err)
	}
	if !containsBackgroundTag(got.Tags, "reviewed") {
		t.Fatalf("managed file did not persist reviewed tag: %v", got.Tags)
	}
	if got.Path != logicalPath || got.StoragePath != storagePath {
		t.Fatalf("managed identity changed: path=%q storage=%q", got.Path, got.StoragePath)
	}
}
