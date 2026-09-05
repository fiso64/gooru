package gooru_test

import (
    "bytes"
    "os"
    "path/filepath"
    "sync"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
    "gooru.local/gooru"
    "gooru.local/internal/encryptedfile"
    "gooru.local/types"
)

func TestClient_RehashFiles(t *testing.T) {
	t.Parallel()

	t.Run("rehash correctly identifies metadata-only change", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange: Add a file and then change only its metadata
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content")
		_, err = client.TagFiles([]string{file1}, []string{"tag1"}, nil, false)
		require.NoError(t, err)
		info, err := os.Stat(file1)
		require.NoError(t, err)
		newModTime := info.ModTime().AddDate(0, -1, 0)
		err = os.Chtimes(file1, newModTime, newModTime)
		require.NoError(t, err)

		// Act: Call rehash and capture the status from its callback
		var status types.RehashStatus
		var mu sync.Mutex
		progressCb := func(path string, s types.RehashStatus, err error) {
			require.NoError(t, err)
			if path == file1 {
				mu.Lock()
				status = s
				mu.Unlock()
			}
		}
		client.RehashFiles([]string{file1}, progressCb, false)

		// Assert: The status must be StatusMetadataUpdated, proving the core logic
		// correctly re-hashed the file, found the hash was the same, and identified
		// the situation as a metadata-only update.
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, types.StatusMetadataUpdated, status)
	})

	// NOTE: Additional tests for RehashFiles could go here, for example:
	// - A full rehash that transfers tags.
	// - Rehash on a file not in the DB.
	// - Rehash on an unchanged file.
}

func TestClient_ProtectedSyncUsesLogicalPlaintextMetadata(t *testing.T) {
	dbPath := setupTestDB(t)
	root := t.TempDir()
	path := createTestFile(t, root, "protected.bin", "logical protected contents")

	plainClient, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	_, err = plainClient.TagFiles([]string{path}, []string{"protected:test"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, plainClient.Close())

	plainInfo, err := os.Stat(path)
	require.NoError(t, err)
	plaintext, err := os.ReadFile(path)
	require.NoError(t, err)
	key := bytes.Repeat([]byte{0x5a}, 32)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
	require.NoError(t, err)
	require.NoError(t, encryptedfile.Encrypt(file, bytes.NewReader(plaintext), int64(len(plaintext)), key))
	require.NoError(t, file.Close())
	// Preserve the logical timestamp while making the physical representation
	// substantially larger than the plaintext. Core sync must not confuse the
	// encrypted container metadata with tracked content metadata.
	require.NoError(t, os.Chtimes(path, plainInfo.ModTime(), plainInfo.ModTime()))
	containerInfo, err := os.Stat(path)
	require.NoError(t, err)
	require.NotEqual(t, plainInfo.Size(), containerInfo.Size())

	client, err := gooru.NewWithOptions(dbPath, false, gooru.OpenOptions{
		Content: gooru.ContentSourceOptions{EncryptionKey: key, ProtectedRoots: []string{root}},
	})
	require.NoError(t, err)
	defer client.Close()

	var status types.RehashStatus
	client.RehashFiles([]string{path}, func(gotPath string, gotStatus types.RehashStatus, err error) {
		require.NoError(t, err)
		if gotPath == path {
			status = gotStatus
		}
	}, true)
	require.Equal(t, types.StatusSkippedUnchanged, status)

	needsRelink, err := client.NeedsRelink([]string{root}, false)
	require.NoError(t, err)
	require.False(t, needsRelink, "logical plaintext metadata should keep protected files stable")
}

func TestClient_Relink(t *testing.T) {
	t.Parallel()

	t.Run("finds file moved into a scanned directory from an outside location", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange:
		// 1. Create two separate directories.
		tempDir := t.TempDir()
		dirA := filepath.Join(tempDir, "dirA")
		dirB := filepath.Join(tempDir, "dirB")
		require.NoError(t, os.Mkdir(dirA, 0755))
		require.NoError(t, os.Mkdir(dirB, 0755))

		// 2. Create and tag a file in the source directory (dirA).
		oldPath := createTestFile(t, dirA, "file.txt", "move me")
		_, err = client.TagFiles([]string{oldPath}, []string{"tag1"}, nil, false)
		require.NoError(t, err)

		// 3. Move the file to the destination directory (dirB).
		newPath := filepath.Join(dirB, "file.txt")
		err = os.Rename(oldPath, newPath)
		require.NoError(t, err)

		// Act (Phase 1 - Pre-check):
		// 4. Run the pre-check on ONLY the destination directory. It MUST detect a change.
		needsScan, err := client.NeedsRelink([]string{dirB}, false)
		require.NoError(t, err)
		assert.True(t, needsScan, "NeedsRelink should detect a change when a file moves into the scanned dir")

		// Act (Phase 2 - Full Scan):
		// 5. Run the full relink scan.
		result, err := client.Relink([]string{dirB})
		require.NoError(t, err)

		// Assert (Dry Run):
		// 6. The result should propose exactly one move.
		require.Len(t, result.ProposedMoves, 1, "Should detect one move")
		assert.Empty(t, result.ProposedAdds, "Should not propose any additions")
		assert.Empty(t, result.ProposedDeletes, "Should not propose any deletions")

		move := result.ProposedMoves[0]
		assert.Equal(t, oldPath, move.OldPath)
		assert.Equal(t, newPath, move.NewLocation.Path)

		// Act (Phase 3 - Apply Changes):
		// 7. Apply the proposed changes.
		stats, err := client.ApplyRelinkChanges(result)
		require.NoError(t, err)

		// Assert (Database State):
		// 8. Check the final state of the database.
		assert.Equal(t, 1, stats.LocationsAdded, "One location should have been updated/added")
		assert.Equal(t, 0, stats.LocationsRemoved)

		tags, status, err := client.GetTagsForFile(newPath, false)
		require.NoError(t, err)
		assert.Equal(t, types.StatusOK, status)
		assert.ElementsMatch(t, []string{"tag1"}, tags)

		// The old path should no longer be in the DB.
		// We expect an os.ErrNotExist because the file is gone from disk, which is what GetTagsForFile checks first.
		_, _, err = client.GetTagsForFile(oldPath, false)
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}
