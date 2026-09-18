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

func TestAnalyzeFileStatesMetadataHeuristicReusesUnchangedHash(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "unchanged.jpg", "unchanged body")
	if _, err := client.TagFiles([]string{path}, []string{"initial"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	analysis, err := client.analyzeFileStates([]string{path}, nil, true)
	if err != nil {
		t.Fatalf("analyze unchanged file: %v", err)
	}
	if len(analysis.allFileData) != 1 {
		t.Fatalf("analyzed files = %d, want 1", len(analysis.allFileData))
	}
	if analysis.allFileData[0].wasModified {
		t.Fatal("unchanged file reported as modified")
	}
	if len(analysis.potentialMoves) != 0 {
		t.Fatalf("unchanged file was hashed despite matching metadata: potential moves=%v", analysis.potentialMoves)
	}
	if len(analysis.locationsToUpsert) != 0 {
		t.Fatalf("unchanged file scheduled a location rewrite: %v", analysis.locationsToUpsert)
	}
}

func TestBackgroundTagMutationPathHeuristicStillDetectsModifiedContent(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "original")
	if _, err := client.TagFiles([]string{path}, []string{"initial"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil || len(files) != 1 {
		t.Fatalf("list files: files=%v err=%v", files, err)
	}
	publicID := client.PublicFileID(files[0].ID)
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:       "add",
		Selector:       map[string][]string{"file_ids": {publicID}},
		Tags:           []string{"reviewed"},
		FileIDs:        []string{publicID},
		FileIDSelector: true,
		MaxPending:     8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}

	if err := os.WriteFile(path, []byte("modified content with a different size"), 0o600); err != nil {
		t.Fatalf("modify file: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationPaths(operation.ID, []string{path}); err != nil {
		t.Fatalf("execute path mutation: %v", err)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read mutation result: found=%v err=%v", found, err)
	}
	if len(state.Notifications) != 1 || state.Notifications[0].Kind != types.NotificationKindModified {
		t.Fatalf("modified-content notification = %+v", state.Notifications)
	}
	if !containsBackgroundTag(state.Notifications[0].OrphanedTags, "initial") {
		t.Fatalf("orphaned tags = %v, want initial", state.Notifications[0].OrphanedTags)
	}
}

func TestBackgroundTagMutationManagedHeuristicStillDetectsModifiedContent(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	logicalPath := filepath.Join(dir, "logical", "photo.jpg")
	storagePath := filepath.Join(dir, "opaque-storage.bin")
	key := bytes.Repeat([]byte{0x71}, 32)
	original := []byte("managed original")
	writeEncryptedTagMutationFile(t, storagePath, original, key)

	resolver, err := filesource.NewProtected(key, []string{dir})
	if err != nil {
		t.Fatalf("create protected source resolver: %v", err)
	}
	client.hasher.SetSourceResolver(resolver)
	metadata, err := client.hasher.FileMetadata(storagePath)
	if err != nil {
		t.Fatalf("read managed metadata: %v", err)
	}
	hash, err := client.hasher.HashSource(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatalf("hash managed plaintext: %v", err)
	}
	if _, err := client.TagKnownFiles([]types.LocationInfo{{
		Path:      logicalPath,
		Hash:      hash,
		Size:      metadata.Size,
		ModTime:   metadata.ModTime.Unix(),
		Extension: filepath.Ext(logicalPath),
	}}, []string{"initial"}, nil); err != nil {
		t.Fatalf("register managed file: %v", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil || len(files) != 1 {
		t.Fatalf("list managed files: files=%v err=%v", files, err)
	}
	publicID := client.PublicFileID(files[0].ID)
	if err := client.SetManagedStoragePath(files[0].ID, storagePath); err != nil {
		t.Fatalf("set managed storage path: %v", err)
	}
	managedFile, err := client.GetFileInfoByPublicID(publicID)
	if err != nil {
		t.Fatalf("load managed file: %v", err)
	}
	managedFile, err = client.ResolveManagedStorage(managedFile)
	if err != nil {
		t.Fatalf("resolve managed storage: %v", err)
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
		t.Fatalf("create managed mutation: %v", err)
	}

	writeEncryptedTagMutationFile(t, storagePath, []byte("managed content changed and is longer"), key)
	if err := client.ExecuteBackgroundTagMutationFiles(operation.ID, []types.FileInfo{managedFile}); err != nil {
		t.Fatalf("execute managed mutation: %v", err)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read managed mutation result: found=%v err=%v", found, err)
	}
	if len(state.Notifications) != 1 || state.Notifications[0].Kind != types.NotificationKindModified {
		t.Fatalf("managed modified-content notification = %+v", state.Notifications)
	}
	if !containsBackgroundTag(state.Notifications[0].OrphanedTags, "initial") {
		t.Fatalf("managed orphaned tags = %v, want initial", state.Notifications[0].OrphanedTags)
	}
}

func writeEncryptedTagMutationFile(t *testing.T, path string, plaintext, key []byte) {
	t.Helper()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("create encrypted managed file: %v", err)
	}
	if err := encryptedfile.Encrypt(out, bytes.NewReader(plaintext), int64(len(plaintext)), key); err != nil {
		_ = out.Close()
		t.Fatalf("encrypt managed file: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close encrypted managed file: %v", err)
	}
}
