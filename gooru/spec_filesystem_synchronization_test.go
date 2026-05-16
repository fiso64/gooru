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

func TestSpecFilesystemSynchronization_EditPathUpdatesTrackedLocation(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	oldPath := createTestFile(t, dir, "before.txt", "same content")
	_, err = client.TagFiles([]string{oldPath}, []string{"project:alpha"}, nil, false)
	require.NoError(t, err)

	newPath := filepath.Join(dir, "after.txt")
	require.NoError(t, os.Rename(oldPath, newPath))

	require.NoError(t, client.EditPath(oldPath, newPath))

	listed, err := client.ListAllFiles()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{newPath}, listed)

	tags, status, err := client.GetTagsForFile(newPath, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusOK, status)
	assert.ElementsMatch(t, []string{"project:alpha"}, tags)
}

func TestSpecFilesystemSynchronization_DuplicateDeletionPreservesRemainingContentTags(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	first := createTestFile(t, dir, "first.txt", "same content")
	second := filepath.Join(dir, "second.txt")
	require.NoError(t, os.WriteFile(second, []byte("same content"), 0644))
	_, err = client.TagFiles([]string{first, second}, []string{"shared"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, os.Remove(first))

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	require.Len(t, result.ProposedDeletes, 1)
	assert.Equal(t, first, result.ProposedDeletes[0].Path)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedAdds)

	stats, err := client.ApplyRelinkChanges(result)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LocationsRemoved)

	tags, status, err := client.GetTagsForFile(second, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusOK, status)
	assert.ElementsMatch(t, []string{"shared"}, tags)
}

func TestSpecFilesystemSynchronization_ModifiedAndMovedFileIsDeletionNotMove(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	oldPath := createTestFile(t, dir, "old.txt", "old content")
	_, err = client.TagFiles([]string{oldPath}, []string{"tag"}, nil, false)
	require.NoError(t, err)

	newPath := filepath.Join(dir, "new.txt")
	require.NoError(t, os.Rename(oldPath, newPath))
	require.NoError(t, os.WriteFile(newPath, []byte("new content"), 0644))

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedAdds)
	require.Len(t, result.ProposedDeletes, 1)
	assert.Equal(t, oldPath, result.ProposedDeletes[0].Path)
}

func TestSpecFilesystemSynchronization_RelinkAddsAllDuplicateLocations(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	tracked := createTestFile(t, dir, "tracked.txt", "same content")
	dupA := filepath.Join(dir, "dup-a.txt")
	dupB := filepath.Join(dir, "dup-b.txt")
	require.NoError(t, os.WriteFile(dupA, []byte("same content"), 0644))
	require.NoError(t, os.WriteFile(dupB, []byte("same content"), 0644))
	_, err = client.TagFiles([]string{tracked}, []string{"shared"}, nil, false)
	require.NoError(t, err)

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	require.Len(t, result.ProposedAdds, 2)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedDeletes)
	assert.ElementsMatch(t, []string{dupA, dupB}, []string{result.ProposedAdds[0].Path, result.ProposedAdds[1].Path})
}
