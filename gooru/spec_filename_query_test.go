package gooru_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
)

func TestBareTagQueryDoesNotMatchFilename(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	filenameMatch := createTestFile(t, dir, "photo-in-filename.jpg", "filename-only bytes")
	tagMatch := createTestFile(t, dir, "plain.jpg", "tagged bytes")

	_, err = client.TagFiles([]string{filenameMatch}, []string{"other"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{tagMatch}, []string{"photo"}, nil, false)
	require.NoError(t, err)

	paths, err := client.ListFilesByQuery("photo", false)
	require.NoError(t, err)
	assert.Equal(t, []string{tagMatch}, paths)
}
