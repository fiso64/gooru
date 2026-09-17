package gooru

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gooru.local/internal/encryptedfile"
	"gooru.local/internal/filesource"
	"gooru.local/types"
)

func TestBackgroundTagMutationManagedFileUsesStoragePath(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	logicalPath := filepath.Join(dir, "logical", "photo.jpg")
	storagePath := filepath.Join(dir, "opaque-storage.bin")
	plaintext := []byte("managed plaintext")
	key := bytes.Repeat([]byte{0x62}, 32)

	out, err := os.OpenFile(storagePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create managed storage: %v", err)
	}
	if err := encryptedfile.Encrypt(out, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		_ = out.Close()
		t.Fatalf("encrypt managed storage: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close managed storage: %v", err)
	}
	resolver, err := filesource.NewProtected(key, []string{dir})
	if err != nil {
		t.Fatalf("create protected source resolver: %v", err)
	}
	client.hasher.SetSourceResolver(resolver)

	metadata, err := client.hasher.FileMetadata(storagePath)
	if err != nil {
		t.Fatalf("read managed metadata: %v", err)
	}
	if metadata.Size != int64(len(plaintext)) {
		t.Fatalf("logical managed size = %d, want %d", metadata.Size, len(plaintext))
	}
	physical, err := os.Stat(storagePath)
	if err != nil {
		t.Fatalf("stat managed storage: %v", err)
	}
	if physical.Size() == metadata.Size {
		t.Fatalf("test did not distinguish encrypted size %d from logical size %d", physical.Size(), metadata.Size)
	}
	hash, err := client.hasher.HashSource(bytes.NewReader(plaintext), int64(len(plaintext)))
	if err != nil {
		t.Fatalf("hash logical managed file: %v", err)
	}
	if _, err := client.TagKnownFiles([]types.LocationInfo{{
		Path:      logicalPath,
		Hash:      hash,
		Size:      metadata.Size,
		ModTime:   metadata.ModTime.Unix(),
		Extension: filepath.Ext(logicalPath),
	}}, []string{"initial"}, nil); err != nil {
		t.Fatalf("register logical file: %v", err)
	}
	if _, err := os.Stat(logicalPath); !os.IsNotExist(err) {
		t.Fatalf("logical path should remain absent, stat err=%v", err)
	}

	files, err := client.GetAllFilesInfo()
	if err != nil || len(files) != 1 {
		t.Fatalf("list logical file: files=%v err=%v", files, err)
	}
	publicID := client.PublicFileID(files[0].ID)
	if publicID == "" {
		t.Fatal("missing public file id")
	}
	if err := client.SetManagedStoragePath(files[0].ID, storagePath); err != nil {
		t.Fatalf("register managed storage: %v", err)
	}
	managedFile, err := client.GetFileInfoByPublicID(publicID)
	if err != nil {
		t.Fatalf("load managed file: %v", err)
	}
	managedFile, err = client.ResolveManagedStorage(managedFile)
	if err != nil {
		t.Fatalf("resolve managed storage: %v", err)
	}
	if managedFile.StoragePath != storagePath {
		t.Fatalf("storage path = %q, want %q", managedFile.StoragePath, storagePath)
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
	if err := client.ExecuteBackgroundTagMutationFiles(operation.ID, []types.FileInfo{managedFile}); err != nil {
		t.Fatalf("execute managed-file mutation: %v", err)
	}

	got, err := client.GetFileInfoByPublicID(publicID)
	if err != nil {
		t.Fatalf("reload managed file: %v", err)
	}
	got, err = client.ResolveManagedStorage(got)
	if err != nil {
		t.Fatalf("resolve reloaded managed file: %v", err)
	}
	if !containsBackgroundTag(got.Tags, "reviewed") {
		t.Fatalf("managed file did not persist reviewed tag: %v", got.Tags)
	}
	if got.Path != logicalPath || got.StoragePath != storagePath {
		t.Fatalf("managed identity changed: path=%q storage=%q", got.Path, got.StoragePath)
	}
}

func TestPopulateOrphanedTagsSharedPreviousHash(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	path := filepath.Join(t.TempDir(), "seed.jpg")
	const previousHash = "shared-previous-hash"
	if _, err := client.TagKnownFiles([]types.LocationInfo{{
		Path:      path,
		Hash:      previousHash,
		Size:      1,
		ModTime:   1,
		Extension: ".jpg",
	}}, []string{"initial"}, nil); err != nil {
		t.Fatalf("seed previous content tags: %v", err)
	}

	data := []fileData{
		{wasModified: true, previousHash: previousHash},
		{wasModified: true, previousHash: previousHash},
		{},
	}
	if err := client.populateOrphanedTags(data); err != nil {
		t.Fatalf("populate orphaned tags: %v", err)
	}
	for index := 0; index < 2; index++ {
		if len(data[index].orphanedTags) != 1 || data[index].orphanedTags[0] != "initial" {
			t.Fatalf("modified file %d orphaned tags = %v, want [initial]", index, data[index].orphanedTags)
		}
	}
	if len(data[2].orphanedTags) != 0 {
		t.Fatalf("unmodified file orphaned tags = %v, want none", data[2].orphanedTags)
	}
}
