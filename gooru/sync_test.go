package gooru_test

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
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
