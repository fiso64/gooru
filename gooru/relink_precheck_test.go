package gooru_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
)

func TestClient_NeedsRelinkIgnoresUnknownSameSizeExtraFile(t *testing.T) {
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	defer client.Close()

	root := t.TempDir()
	trackedPath := createTestFile(t, root, "tracked.txt", "known123")
	_, err = client.TagFiles([]string{trackedPath}, []string{"tag1"}, nil, false)
	require.NoError(t, err)

	createTestFile(t, root, "unknown.txt", "other456")

	needsRelink, err := client.NeedsRelink([]string{root}, false)
	require.NoError(t, err)
	assert.False(t, needsRelink, "an unrelated same-size file should not make the pre-check disagree with Relink")

	result, err := client.Relink([]string{root})
	require.NoError(t, err)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedAdds)
	assert.Empty(t, result.ProposedDeletes)
}

func TestClient_NeedsRelinkDetectsKnownDuplicateExtraFile(t *testing.T) {
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	defer client.Close()

	root := t.TempDir()
	trackedPath := createTestFile(t, root, "tracked.txt", "known123")
	_, err = client.TagFiles([]string{trackedPath}, []string{"tag1"}, nil, false)
	require.NoError(t, err)

	duplicatePath := createTestFile(t, root, "duplicate.txt", "known123")

	needsRelink, err := client.NeedsRelink([]string{root}, false)
	require.NoError(t, err)
	assert.True(t, needsRelink, "a same-content extra location should still require relinking")

	result, err := client.Relink([]string{root})
	require.NoError(t, err)
	require.Len(t, result.ProposedAdds, 1)
	assert.Equal(t, duplicatePath, result.ProposedAdds[0].Path)
	assert.Empty(t, result.ProposedMoves)
	assert.Empty(t, result.ProposedDeletes)
}
