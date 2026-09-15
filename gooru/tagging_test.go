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
		callbackCount := 0
		var mu sync.Mutex

		progressCb := func(filePath string, err error) {
			mu.Lock()
			defer mu.Unlock()
			callbackCount++
			if err != nil {
				progressErr = err
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
		assert.Equal(t, 1, callbackCount, "A missing file should be reported exactly once")
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

func TestClient_SetTagsForFiles(t *testing.T) {
	t.Parallel()

	t.Run("replace all existing tags with a new set", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content A")
		_, err = client.TagFiles([]string{file1}, []string{"old:tag", "another"}, nil, false)
		require.NoError(t, err)

		// Act
		result, err := client.SetTagsForFiles([]string{file1}, []string{"new:tag", "final"}, nil, false)
		require.NoError(t, err)

		// Assert
		// AffectedCount for set is (cleared + added) = 2 + 2 = 4
		assert.Equal(t, 4, result.AffectedCount)
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.ElementsMatch(t, []string{"new:tag", "final"}, tags)
	})

	t.Run("setting empty tags removes all tags", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content A")
		_, err = client.TagFiles([]string{file1}, []string{"tag1", "tag2"}, nil, false)
		require.NoError(t, err)

		// Act
		result, err := client.SetTagsForFiles([]string{file1}, []string{}, nil, false)
		require.NoError(t, err)

		// Assert
		// AffectedCount for set is (cleared + added) = 2 + 0 = 2
		assert.Equal(t, 2, result.AffectedCount)
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.Empty(t, tags)
	})
}

func TestClient_UntagFiles(t *testing.T) {
	t.Parallel()

	t.Run("remove specific tags from a file", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content A")
		_, err = client.TagFiles([]string{file1}, []string{"tag1", "tag2", "tag3"}, nil, false)
		require.NoError(t, err)

		// Act
		result, err := client.UntagFiles([]string{file1}, []string{"tag1", "tag3"}, nil, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 2, result.AffectedCount)
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.ElementsMatch(t, []string{"tag2"}, tags)
	})

	t.Run("removing a non-existent tag is a no-op", func(t *testing.T) {
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
		result, err := client.UntagFiles([]string{file1}, []string{"non-existent"}, nil, false)
		require.NoError(t, err)

		// Assert
		assert.Equal(t, 0, result.AffectedCount)
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.ElementsMatch(t, []string{"tag1"}, tags)
	})

	t.Run("untag with empty tags list removes all tags", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		file1 := createTestFile(t, tempDir, "file1.txt", "content A")
		_, err = client.TagFiles([]string{file1}, []string{"tag1", "tag2"}, nil, false)
		require.NoError(t, err)

		// Act: This is a special case that maps to SetTags with empty tags
		result, err := client.UntagFiles([]string{file1}, []string{}, nil, false)
		require.NoError(t, err)

		// Assert
		// The underlying SetTags returns (cleared + added) = 2 + 0 = 2
		assert.Equal(t, 2, result.AffectedCount)
		tags, _, _ := client.GetTagsForFile(file1, false)
		assert.Empty(t, tags)
	})
}

func TestClient_TagFilesByQuery(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	// Arrange
	tempDir := t.TempDir()
	f1 := createTestFile(t, tempDir, "f1.txt", "c1")
	f2 := createTestFile(t, tempDir, "f2.txt", "c2")
	f3 := createTestFile(t, tempDir, "f3.txt", "c3")
	_, err = client.TagFiles([]string{f1}, []string{"project:a", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{f2}, []string{"project:b", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{f3}, []string{"project:a", "final"}, nil, false)
	require.NoError(t, err)

	// Act: Tag all files with `review` tag with a new `archived` tag.
	affected, err := client.TagFilesByQuery("review", []string{"archived"})
	require.NoError(t, err)

	// Assert: f1 and f2 should be affected.
	assert.Equal(t, 2, affected)
	tags1, _, _ := client.GetTagsForFile(f1, false)
	assert.Contains(t, tags1, "archived")
	tags2, _, _ := client.GetTagsForFile(f2, false)
	assert.Contains(t, tags2, "archived")
	tags3, _, _ := client.GetTagsForFile(f3, false)
	assert.NotContains(t, tags3, "archived")
}

func TestClient_SetTagsForFilesByQuery(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	// Arrange
	tempDir := t.TempDir()
	f1 := createTestFile(t, tempDir, "f1.txt", "c1")
	f2 := createTestFile(t, tempDir, "f2.txt", "c2")
	f3 := createTestFile(t, tempDir, "f3.txt", "c3")
	_, err = client.TagFiles([]string{f1}, []string{"project:a", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{f2}, []string{"project:b", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{f3}, []string{"project:a", "final"}, nil, false)
	require.NoError(t, err)

	// Act: Replace tags for all files with `project:a` with a single `migrated` tag.
	affected, err := client.SetTagsForFilesByQuery("project:a", []string{"migrated"})
	require.NoError(t, err)

	// Assert: f1 and f3 should be affected.
	assert.Equal(t, 2, affected)
	tags1, _, _ := client.GetTagsForFile(f1, false)
	assert.Equal(t, []string{"migrated"}, tags1)
	tags3, _, _ := client.GetTagsForFile(f3, false)
	assert.Equal(t, []string{"migrated"}, tags3)
	// f2 should be unchanged
	tags2, _, _ := client.GetTagsForFile(f2, false)
	assert.Contains(t, tags2, "review")
}

func TestClient_UntagFilesByQuery(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	// Arrange
	tempDir := t.TempDir()
	f1 := createTestFile(t, tempDir, "f1.txt", "c1")
	f2 := createTestFile(t, tempDir, "f2.txt", "c2")
	f3 := createTestFile(t, tempDir, "f3.txt", "c3")
	_, err = client.TagFiles([]string{f1}, []string{"project:a", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{f2}, []string{"project:b", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{f3}, []string{"project:a", "final"}, nil, false)
	require.NoError(t, err)

	// Act: Remove the `review` tag from all files.
	affected, err := client.UntagFilesByQuery("review", []string{"review"})
	require.NoError(t, err)

	// Assert: f1 and f2 had the tag in the initial state. This test is now independent.
	assert.Equal(t, 2, affected)
	tags1, _, _ := client.GetTagsForFile(f1, false)
	assert.NotContains(t, tags1, "review")
	tags2, _, _ := client.GetTagsForFile(f2, false)
	assert.NotContains(t, tags2, "review")
	// f3 should be unchanged
	tags3, _, _ := client.GetTagsForFile(f3, false)
	assert.Contains(t, tags3, "final")
}

func TestClient_RenameTag(t *testing.T) {
	t.Parallel()

	t.Run("successfully renames a tag and updates cache", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		f1 := createTestFile(t, tempDir, "f1.txt", "c1")
		f2 := createTestFile(t, tempDir, "f2.txt", "c2")
		_, err = client.TagFiles([]string{f1, f2}, []string{"project:alpha", "version:1"}, nil, false)
		require.NoError(t, err)

		// Act
		err = client.RenameTag("project:alpha", "project:beta")
		require.NoError(t, err)

		// Assert
		tags1, _, _ := client.GetTagsForFile(f1, false)
		assert.ElementsMatch(t, []string{"project:beta", "version:1"}, tags1)

		tags2, _, _ := client.GetTagsForFile(f2, false)
		assert.ElementsMatch(t, []string{"project:beta", "version:1"}, tags2)

		// Assert cache is updated
		info1, _, _ := client.GetFileInfoForFile(f1, false)
		assert.Contains(t, info1.Tags, "project:beta")
		assert.NotContains(t, info1.Tags, "project:alpha")
	})

	t.Run("fail to rename to an existing tag", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange
		tempDir := t.TempDir()
		f1 := createTestFile(t, tempDir, "f1.txt", "c1")
		_, err = client.TagFiles([]string{f1}, []string{"old_tag", "existing_tag"}, nil, false)
		require.NoError(t, err)

		// Act
		err = client.RenameTag("old_tag", "existing_tag")

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("fail to rename a non-existent tag", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Act
		err = client.RenameTag("non-existent", "new_name")

		// Assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}
