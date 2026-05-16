package gooru_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
)

func TestSpecTagValidationAndQuerySemantics(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	photoAlbum := createTestFile(t, dir, "album.jpg", "jpg bytes")
	photoSimple := createTestFile(t, dir, "simple.png", "png bytes")
	doc := createTestFile(t, dir, "doc.txt", "doc bytes")

	_, err = client.TagFiles([]string{photoAlbum}, []string{"photo:album1", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{photoSimple}, []string{"photo", "review"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{doc}, []string{"project:alpha"}, nil, false)
	require.NoError(t, err)

	invalidTags := [][]string{{"my tag"}, {"!important"}, {"final-"}, {":work"}, {"project:v1:"}, {"ext:backup"}, {"type:document"}, {"project:*"}, {"@tagged"}}
	for _, tags := range invalidTags {
		_, err := client.TagFiles([]string{doc}, tags, nil, false)
		assert.Error(t, err, tags)
	}

	paths, err := client.ListFilesByQuery("photo", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{photoAlbum, photoSimple}, paths)

	paths, err = client.ListFilesByQuery("photo:", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{photoSimple}, paths)

	paths, err = client.ListFilesByQuery("photo:*", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{photoAlbum}, paths)

	paths, err = client.ListFilesByQuery("ext:jpg", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{photoAlbum}, paths)

	paths, err = client.ListFilesByQuery("type:img -photo:album1", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{photoSimple}, paths)

	paths, err = client.ListFilesByQuery("@tagged", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{photoAlbum, photoSimple, doc}, paths)
}
