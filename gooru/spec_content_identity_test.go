package gooru_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

func TestSpecContentIdentity_DuplicatesShareTags(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	original := createTestFile(t, dir, "original.txt", "same content")
	duplicate := filepath.Join(dir, "duplicate.txt")
	require.NoError(t, os.WriteFile(duplicate, []byte("same content"), 0644))

	_, err = client.TagFiles([]string{original}, []string{"project:alpha"}, nil, false)
	require.NoError(t, err)
	result, err := client.TagFiles([]string{duplicate}, []string{"copied"}, nil, false)
	require.NoError(t, err)

	assert.Equal(t, 1, result.AffectedCount)
	assert.Empty(t, result.Notifications)

	for _, path := range []string{original, duplicate} {
		tags, status, err := client.GetTagsForFile(path, false)
		require.NoError(t, err)
		assert.Equal(t, types.StatusOK, status)
		assert.ElementsMatch(t, []string{"project:alpha", "copied"}, tags)
	}
}

func TestSpecContentIdentity_TaggingMovedFileUpdatesTrackedPath(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	oldPath := createTestFile(t, dir, "photo.jpg", "image bytes")
	_, err = client.TagFiles([]string{oldPath}, []string{"vacation", "summer"}, nil, false)
	require.NoError(t, err)

	newPath := filepath.Join(dir, "holiday.jpg")
	require.NoError(t, os.Rename(oldPath, newPath))

	result, err := client.TagFiles([]string{newPath}, []string{"trip"}, nil, false)
	require.NoError(t, err)
	require.Len(t, result.Notifications, 1)
	assert.Equal(t, types.NotificationKindMoveDetected, result.Notifications[0].Kind)
	assert.Equal(t, oldPath, result.Notifications[0].OldPath)
	assert.Equal(t, newPath, result.Notifications[0].NewPath)

	listed, err := client.ListAllFiles()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{newPath}, listed)

	tags, status, err := client.GetTagsForFile(newPath, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusOK, status)
	assert.ElementsMatch(t, []string{"vacation", "summer", "trip"}, tags)
}
