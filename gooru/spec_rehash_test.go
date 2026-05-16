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

func TestSpecRehash_TransfersTagsToModifiedContent(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	file := createTestFile(t, dir, "report.txt", "draft")
	_, err = client.TagFiles([]string{file}, []string{"project", "draft"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(file, []byte("final"), 0644))

	statuses := collectRehashStatuses(t, client, []string{file}, false)
	assert.Equal(t, types.StatusRehashed, statuses[file])

	tags, status, err := client.GetTagsForFile(file, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusOK, status)
	assert.ElementsMatch(t, []string{"project", "draft"}, tags)
}

func TestSpecRehash_SkipsUnchangedAndUnknownFiles(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	tracked := createTestFile(t, dir, "tracked.txt", "content")
	unknown := createTestFile(t, dir, "unknown.txt", "content")
	_, err = client.TagFiles([]string{tracked}, []string{"tag"}, nil, false)
	require.NoError(t, err)

	statuses := collectRehashStatuses(t, client, []string{tracked, unknown}, false)
	assert.Equal(t, types.StatusSkippedUnchanged, statuses[tracked])
	assert.Equal(t, types.StatusSkippedNotInDB, statuses[unknown])
}

func TestSpecRehash_DeletedTrackedFileReportsError(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	file := createTestFile(t, dir, "deleted.txt", "content")
	_, err = client.TagFiles([]string{file}, []string{"tag"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, os.Remove(file))

	errs := collectRehashErrors(t, client, []string{file}, false)
	require.Error(t, errs[file])
	assert.ErrorIs(t, errs[file], os.ErrNotExist)
}

func TestSpecRehash_MergesTagsWhenNewContentAlreadyExists(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	a := createTestFile(t, dir, "a.txt", "alpha")
	b := createTestFile(t, dir, "b.txt", "beta")
	_, err = client.TagFiles([]string{a}, []string{"from:a"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{b}, []string{"from:b"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(a, []byte("beta"), 0644))

	statuses := collectRehashStatuses(t, client, []string{a}, false)
	assert.Equal(t, types.StatusRehashed, statuses[a])

	for _, path := range []string{a, b} {
		tags, status, err := client.GetTagsForFile(path, false)
		require.NoError(t, err)
		assert.Equal(t, types.StatusOK, status)
		assert.ElementsMatch(t, []string{"from:a", "from:b"}, tags)
	}
}

func collectRehashStatuses(t *testing.T, client *gooru.Client, paths []string, useMetadata bool) map[string]types.RehashStatus {
	t.Helper()
	statuses := make(map[string]types.RehashStatus)
	client.RehashFiles(paths, func(path string, status types.RehashStatus, err error) {
		require.NoError(t, err)
		statuses[path] = status
	}, useMetadata)
	return statuses
}

func collectRehashErrors(t *testing.T, client *gooru.Client, paths []string, useMetadata bool) map[string]error {
	t.Helper()
	errs := make(map[string]error)
	client.RehashFiles(paths, func(path string, status types.RehashStatus, err error) {
		errs[path] = err
	}, useMetadata)
	return errs
}

func TestSpecRelink_DetectsMoveWithinScannedDirectory(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	oldPath := createTestFile(t, dir, "old.txt", "move within")
	_, err = client.TagFiles([]string{oldPath}, []string{"tag"}, nil, false)
	require.NoError(t, err)
	newPath := filepath.Join(dir, "new.txt")
	require.NoError(t, os.Rename(oldPath, newPath))

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	require.Len(t, result.ProposedMoves, 1)
	assert.Equal(t, oldPath, result.ProposedMoves[0].OldPath)
	assert.Equal(t, newPath, result.ProposedMoves[0].NewLocation.Path)
	assert.Empty(t, result.ProposedAdds)
	assert.Empty(t, result.ProposedDeletes)

	stats, err := client.ApplyRelinkChanges(result)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LocationsAdded)

	tags, status, err := client.GetTagsForFile(newPath, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusOK, status)
	assert.ElementsMatch(t, []string{"tag"}, tags)
}

func TestSpecRelink_DetectsDuplicateLocation(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	tracked := createTestFile(t, dir, "tracked.txt", "same")
	duplicate := filepath.Join(dir, "duplicate.txt")
	_, err = client.TagFiles([]string{tracked}, []string{"tag"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(duplicate, []byte("same"), 0644))

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	require.Len(t, result.ProposedAdds, 1)
	assert.Equal(t, duplicate, result.ProposedAdds[0].Path)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedDeletes)

	stats, err := client.ApplyRelinkChanges(result)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LocationsAdded)

	tags, status, err := client.GetTagsForFile(duplicate, false)
	require.NoError(t, err)
	assert.Equal(t, types.StatusOK, status)
	assert.ElementsMatch(t, []string{"tag"}, tags)
}

func TestSpecRelink_DoesNotAddBrandNewContent(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	tracked := createTestFile(t, dir, "tracked.txt", "known")
	_ = createTestFile(t, dir, "brand-new.txt", "alien")
	_, err = client.TagFiles([]string{tracked}, []string{"tag"}, nil, false)
	require.NoError(t, err)

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedAdds)
	assert.Empty(t, result.ProposedDeletes)
}

func TestSpecRelink_ProposesDeletedLocation(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	file := createTestFile(t, dir, "gone.txt", "content")
	_, err = client.TagFiles([]string{file}, []string{"tag"}, nil, false)
	require.NoError(t, err)
	require.NoError(t, os.Remove(file))

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	require.Len(t, result.ProposedDeletes, 1)
	assert.Equal(t, file, result.ProposedDeletes[0].Path)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedAdds)

	stats, err := client.ApplyRelinkChanges(result)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LocationsRemoved)
}
