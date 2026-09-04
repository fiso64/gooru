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

func TestFilenameContainsUsesTrackedFilenameWorkflow(t *testing.T) {
	t.Parallel()
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	wanted := createTestFile(t, dir, "Summer-Trip_100%.JPG", "wanted bytes")
	other := createTestFile(t, dir, "plain.jpg", "other bytes")

	_, err = client.TagFiles([]string{wanted, other}, []string{"tracked"}, nil, false)
	require.NoError(t, err)

	for _, test := range []struct {
		query string
		want  []string
	}{
		{query: `"@filename_contains:trip_100%"`, want: []string{wanted}},
		{query: `@filename_contains:TRIP`, want: []string{wanted}},
		{query: `@filename_contains:ip`, want: []string{wanted}},
		{query: `@filename_contains:missing`, want: nil},
	} {
		paths, err := client.ListFilesByQuery(test.query, false)
		require.NoError(t, err, "query %s", test.query)
		assert.Equal(t, test.want, paths, "query %s", test.query)
	}
}
