package gooru_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

// setupTestDB is a test helper that creates a temporary, initialized database for a test.
// It returns the path to the database file.
// The testing framework's `t.TempDir()` ensures the parent directory is cleaned up automatically.
func setupTestDB(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")
	err := gooru.Init(dbPath, types.StrategyPartial, false)
	require.NoError(t, err, "Failed to initialize test database")
	return dbPath
}

// createTestFile is a helper to create a file with specific content in a directory.
func createTestFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err, "Failed to create test file")
	return filePath
}

func TestClient_TagFiles(t *testing.T) {
	t.Parallel()

	t.Run("tag new files successfully", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content A")
		file2 := createTestFile(t, tempDir, "file2.txt", "content B")

		// Act
		result, err := client.TagFiles([]string{file1, file2}, []string{"tag1", "project:alpha"}, nil, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 4, result.AffectedCount, "Expected 2 tags for 2 files")
		assert.Empty(t, result.Notifications, "Should be no notifications for new files")

		tags1, status1, err1 := client.GetTagsForFile(file1, false)
		require.NoError(t, err1)
		assert.Equal(t, types.StatusOK, status1)
		assert.ElementsMatch(t, []string{"tag1", "project:alpha"}, tags1)

		tags2, status2, err2 := client.GetTagsForFile(file2, false)
		require.NoError(t, err2)
		assert.Equal(t, types.StatusOK, status2)
		assert.ElementsMatch(t, []string{"tag1", "project:alpha"}, tags2)
	})

	t.Run("add new tags to existing file", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content A")
		_, err = client.TagFiles([]string{file1}, []string{"initial"}, nil, false)
		require.NoError(t, err)

		// Act
		result, err := client.TagFiles([]string{file1}, []string{"additional"}, nil, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 1, result.AffectedCount)
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.ElementsMatch(t, []string{"initial", "additional"}, tags)
	})

	t.Run("re-applying existing tag is a no-op", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content A")
		_, err = client.TagFiles([]string{file1}, []string{"tag1"}, nil, false)
		require.NoError(t, err)

		// Act
		result, err := client.TagFiles([]string{file1}, []string{"tag1"}, nil, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 0, result.AffectedCount, "Re-applying a tag should affect 0 new associations")
	})

	t.Run("tagging a modified file updates record and orphans old tags (default behavior)", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange: Add a file with an initial tag
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "original content")
		_, err = client.TagFiles([]string{file1}, []string{"initial_tag"}, nil, false)
		require.NoError(t, err)

		// Modify the file on disk. This test relies on the content change being detectable.
		// The new "always hash" default guarantees this.
		err = os.WriteFile(file1, []byte("modified content"), 0644)
		require.NoError(t, err)

		// Act: Tag the modified file using the default (no heuristic)
		result, err := client.TagFiles([]string{file1}, []string{"new_tag"}, nil, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 1, result.AffectedCount)
		require.Len(t, result.Notifications, 1, "Expected one notification for the modification")

		notification := result.Notifications[0]
		assert.Equal(t, types.NotificationKindModified, notification.Kind)
		assert.Equal(t, file1, notification.OriginalPath)
		assert.ElementsMatch(t, []string{"initial_tag"}, notification.OrphanedTags)

		// The file should now only have the new tag associated with its new content
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.ElementsMatch(t, []string{"new_tag"}, tags)
	})

	t.Run("tagging a moved file detects the move", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange: Add an original file and then move it
		tempDir := t.TempDir()
		oldPath := createTestFile(t, tempDir, "old.txt", "consistent content")
		_, err = client.TagFiles([]string{oldPath}, []string{"initial_tag"}, nil, false)
		require.NoError(t, err)

		newPath := filepath.Join(tempDir, "new.txt")
		err = os.Rename(oldPath, newPath)
		require.NoError(t, err)

		// Act: Tag the file at its new path
		result, err := client.TagFiles([]string{newPath}, []string{"new_tag"}, nil, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 1, result.AffectedCount)
		require.Len(t, result.Notifications, 1, "Expected one notification for the move")

		notification := result.Notifications[0]
		assert.Equal(t, types.NotificationKindMoveDetected, notification.Kind)
		assert.Equal(t, newPath, notification.OriginalPath)
		assert.Equal(t, oldPath, notification.OldPath)
		assert.Equal(t, newPath, notification.NewPath)

		// The file at the new path should have both the old and new tags
		tags, _, _ := client.GetTagsForFile(newPath, false)
		assert.ElementsMatch(t, []string{"initial_tag", "new_tag"}, tags)
	})

	t.Run("tagging non-existent file reports error via callback", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		nonExistentPath := "/path/to/a/file/that/does/not/exist.txt"
		var progressErr error
		var mu sync.Mutex

		progressCb := func(filePath string, err error) {
			if err != nil {
				mu.Lock()
				progressErr = err
				mu.Unlock()
			}
		}

		// Act
		result, err := client.TagFiles([]string{nonExistentPath}, []string{"tag1"}, progressCb, false)
		require.NoError(t, err, "The main function should not fail for a missing file")

		// Assert
		assert.Zero(t, result.AffectedCount)
		assert.Empty(t, result.Notifications)
		mu.Lock()
		defer mu.Unlock()
		require.Error(t, progressErr, "Expected an error from the progress callback")
		assert.ErrorIs(t, progressErr, os.ErrNotExist, "The error should be a 'not exist' error")
	})

	t.Run("tagging mixed valid and invalid files processes valid ones", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		validFile := createTestFile(t, tempDir, "valid.txt", "content")
		nonExistentFile := filepath.Join(tempDir, "nonexistent.txt")

		errorMap := make(map[string]error)
		var mu sync.Mutex
		progressCb := func(filePath string, err error) {
			mu.Lock()
			defer mu.Unlock()
			errorMap[filePath] = err
		}

		// Act
		result, err := client.TagFiles([]string{validFile, nonExistentFile}, []string{"tag1"}, progressCb, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 1, result.AffectedCount)

		// Check progress callback results
		mu.Lock()
		defer mu.Unlock()
		assert.Nil(t, errorMap[validFile], "Valid file should not have an error")
		assert.Error(t, errorMap[nonExistentFile], "Non-existent file should have an error")

		// Check DB state
		tags, _, _ := client.GetTagsForFile(validFile, false)
		assert.ElementsMatch(t, []string{"tag1"}, tags)
	})

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

	t.Run("tagging with heuristic handles metadata-only change correctly", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange: Add a file, then change only its modtime
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content")
		_, err = client.TagFiles([]string{file1}, []string{"initial_tag"}, nil, false)
		require.NoError(t, err)
		info, err := os.Stat(file1)
		require.NoError(t, err)
		newModTime := info.ModTime().AddDate(0, -1, 0)
		err = os.Chtimes(file1, newModTime, newModTime)
		require.NoError(t, err)

		// Act: Tag the file using the metadata heuristic. This should trigger a re-hash,
		// find the content is the same, and proceed with a normal additive tag operation.
		result, err := client.TagFiles([]string{file1}, []string{"new_tag"}, nil, true)
		require.NoError(t, err)

		// Assert: Because the content hash was ultimately the same, no "modified"
		// notification should be generated and no tags should be orphaned.
		assert.Empty(t, result.Notifications, "No notification should be generated for a metadata-only change")
		assert.Equal(t, 1, result.AffectedCount)

		// Both tags should now be present.
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.ElementsMatch(t, []string{"initial_tag", "new_tag"}, tags)
	})

	t.Run("tagging with metadata heuristic does NOT re-hash unchanged file", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange: Add a file with an initial tag
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content")
		_, err = client.TagFiles([]string{file1}, []string{"initial_tag"}, nil, false)
		require.NoError(t, err)

		// Act: Tag the file again using the metadata heuristic. The file is unchanged.
		result, err := client.TagFiles([]string{file1}, []string{"new_tag"}, nil, true)
		require.NoError(t, err)

		// Assert: No modification should be detected, so no notifications.
		assert.Empty(t, result.Notifications, "No notification should be generated for an unchanged file")
		assert.Equal(t, 1, result.AffectedCount)

		// The file should now have both tags.
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.ElementsMatch(t, []string{"initial_tag", "new_tag"}, tags)
	})
}
