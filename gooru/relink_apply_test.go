package gooru_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

func TestRelinkApplyReplacesModifiedPathWithKnownContent(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	changed := createTestFile(t, dir, "changed.txt", "alpha")
	known := createTestFile(t, dir, "known.txt", "bravo")
	_, err = client.TagFiles([]string{changed}, []string{"from:alpha"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{known}, []string{"from:bravo"}, nil, false)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(changed, []byte("bravo"), 0644))

	result, err := client.Relink([]string{dir})
	require.NoError(t, err)
	assert.Empty(t, result.ProposedMoves)
	require.Len(t, result.ProposedDeletes, 1)
	assert.Equal(t, changed, result.ProposedDeletes[0].Path)
	require.Len(t, result.ProposedAdds, 1)
	assert.Equal(t, changed, result.ProposedAdds[0].Path)

	stats, err := client.ApplyRelinkChanges(result)
	require.NoError(t, err)
	assert.Equal(t, 1, stats.LocationsAdded)
	assert.Equal(t, 1, stats.LocationsRemoved)

	listed, err := client.ListAllFiles()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{changed, known}, listed)

	for _, path := range []string{changed, known} {
		tags, status, err := client.GetTagsForFile(path, false)
		require.NoError(t, err)
		assert.Equal(t, types.StatusOK, status)
		assert.ElementsMatch(t, []string{"from:bravo"}, tags)
	}
}
