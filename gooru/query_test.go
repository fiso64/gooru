package gooru_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

func TestClient_GetTagsForFile_MovedFile(t *testing.T) {
	t.Parallel()

	// This test case is designed to fail with the old path-centric logic
	// and pass with the new content-centric logic.
	t.Run("gettags on a moved file should find tags by content hash", func(t *testing.T) {
		t.Parallel()
		dbPath := setupTestDB(t)
		client, err := gooru.New(dbPath, false)
		require.NoError(t, err)
		t.Cleanup(func() { client.Close() })

		// Arrange:
		// 1. Create and tag a file at an original location.
		tempDir := t.TempDir()
		oldPath := createTestFile(t, tempDir, "original.txt", "move-me-content")
		_, err = client.TagFiles([]string{oldPath}, []string{"project:kestrel", "status:final"}, nil, false)
		require.NoError(t, err, "Setup: failed to tag original file")

		// 2. Move the file to a new, untracked location.
		newPath := filepath.Join(tempDir, "moved.txt")
		err = os.Rename(oldPath, newPath)
		require.NoError(t, err, "Setup: failed to move file")

		// Act:
		// 3. Call GetTagsForFile on the NEW path.
		tags, status, err := client.GetTagsForFile(newPath, false) // `false` for default, content-hashing behavior

		// Assert:
		// 4. The call must succeed.
		require.NoError(t, err)

		// 5. The status must correctly identify it as a file with known content at an untracked path.
		//    (The old code would return StatusNotInDB).
		assert.Equal(t, types.StatusUntrackedContent, status, "Status should be 'UntrackedContent'")

		// 6. Most importantly, it should find the tags associated with the file's content.
		//    (The old code would return an empty slice).
		assert.ElementsMatch(t, []string{"project:kestrel", "status:final"}, tags, "Should retrieve tags for the moved file")
	})
}

func TestClient_CountAndExistsInvalidQueryWrapSentinel(t *testing.T) {
	t.Parallel()

	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	_, err = client.CountFilesByQuery("(", false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, gooru.ErrInvalidQuery), "CountFilesByQuery should wrap ErrInvalidQuery")

	_, err = client.ExistsFilesByQuery("(", false)
	require.Error(t, err)
	assert.True(t, errors.Is(err, gooru.ErrInvalidQuery), "ExistsFilesByQuery should wrap ErrInvalidQuery")
}
