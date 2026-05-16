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

func TestSpecInformationRetrieval_GetTagsStatuses(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	tracked := createTestFile(t, dir, "tracked.txt", "tracked content")
	modified := createTestFile(t, dir, "modified.txt", "old bytes")
	movedOld := createTestFile(t, dir, "moved-old.txt", "moved content")
	unknown := createTestFile(t, dir, "unknown.txt", "unknown content")

	_, err = client.TagFiles([]string{tracked}, []string{"state:ok"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{modified}, []string{"state:modified"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{movedOld}, []string{"state:moved"}, nil, false)
	require.NoError(t, err)

	overwriteFilePreservingMetadata(t, modified, "new bytes")
	movedNew := filepath.Join(dir, "moved-new.txt")
	require.NoError(t, os.Rename(movedOld, movedNew))

	cases := []struct {
		path   string
		status types.FileStatus
		tags   []string
	}{
		{tracked, types.StatusOK, []string{"state:ok"}},
		{modified, types.StatusModified, []string{}},
		{movedNew, types.StatusUntrackedContent, []string{"state:moved"}},
		{unknown, types.StatusNotInDB, []string{}},
	}

	for _, tc := range cases {
		tags, status, err := client.GetTagsForFile(tc.path, false)
		require.NoError(t, err)
		assert.Equal(t, tc.status, status, tc.path)
		assert.ElementsMatch(t, tc.tags, tags, tc.path)
	}
}

func TestSpecInformationRetrieval_MetadataOptInIsPathCentric(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	file := createTestFile(t, dir, "same-metadata.txt", "abcdef")
	_, err = client.TagFiles([]string{file}, []string{"old"}, nil, false)
	require.NoError(t, err)
	overwriteFilePreservingMetadata(t, file, "uvwxyz")

	tags, status, err := client.GetTagsForFile(file, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusModified, status)
	assert.Empty(t, tags)

	tags, status, err = client.GetTagsForFile(file, true)
	require.NoError(t, err)
	assert.Equal(t, types.StatusOK, status)
	assert.ElementsMatch(t, []string{"old"}, tags)
}

func TestSpecInformationRetrieval_ExpressionReadsDatabaseStateOnly(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	oldPath := createTestFile(t, dir, "before.txt", "content")
	_, err = client.TagFiles([]string{oldPath}, []string{"listed"}, nil, false)
	require.NoError(t, err)

	newPath := filepath.Join(dir, "after.txt")
	require.NoError(t, os.Rename(oldPath, newPath))

	paths, err := client.ListFilesByQuery("listed", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{oldPath}, paths)

	_, status, err := client.GetTagsForFile(newPath, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusUntrackedContent, status)
}
