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

func TestSpecRehash_PreservesTagsOnUnchangedDuplicateLocation(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	modified := createTestFile(t, dir, "modified.txt", "shared content")
	_, err = client.TagFiles([]string{modified}, []string{"keep"}, nil, false)
	require.NoError(t, err)

	unchanged := filepath.Join(dir, "unchanged.txt")
	require.NoError(t, os.WriteFile(unchanged, []byte("shared content"), 0644))

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	require.Len(t, result.ProposedAdds, 1)
	assert.Equal(t, unchanged, result.ProposedAdds[0].Path)
	_, err = client.ApplyRelinkChanges(result)
	require.NoError(t, err)

	for _, path := range []string{modified, unchanged} {
		tags, status, err := client.GetTagsForFile(path, false)
		require.NoError(t, err)
		require.Equal(t, types.StatusOK, status)
		assert.ElementsMatch(t, []string{"keep"}, tags)
	}

	require.NoError(t, os.WriteFile(modified, []byte("changed content"), 0644))
	statuses := collectRehashStatuses(t, client, []string{modified}, false)
	require.Equal(t, types.StatusRehashed, statuses[modified])

	for _, path := range []string{modified, unchanged} {
		tags, status, err := client.GetTagsForFile(path, false)
		require.NoError(t, err)
		require.Equal(t, types.StatusOK, status)
		assert.ElementsMatch(t, []string{"keep"}, tags)
	}
}
