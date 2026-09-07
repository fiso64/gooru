package gooru_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/gooru"
	"gooru.local/types"
)

func facetsByKind(facets []types.TagWithCount) map[string]int {
	out := make(map[string]int, len(facets))
	for _, facet := range facets {
		out[facet.Tag] = facet.Count
	}
	return out
}

func TestKindFacetsByQueryPureTagExclusions(t *testing.T) {
	dbPath := setupTestDB(t)
	client, err := gooru.New(dbPath, false)
	require.NoError(t, err)
	t.Cleanup(func() { client.Close() })

	dir := t.TempDir()
	visible := createTestFile(t, dir, "visible.jpg", "visible")
	hidden := createTestFile(t, dir, "hidden.jpg", "hidden")
	overlap := createTestFile(t, dir, "overlap.mp4", "overlap")

	_, err = client.TagFiles([]string{visible}, []string{"visible"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{hidden}, []string{"hidden"}, nil, false)
	require.NoError(t, err)
	_, err = client.TagFiles([]string{overlap}, []string{"hidden", "secret:private"}, nil, false)
	require.NoError(t, err)

	facets, err := client.KindFacetsByQuery("-hidden", false)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"photo": 1}, facetsByKind(facets))

	// The video carries both exclusions. It must be removed once, not twice.
	facets, err = client.KindFacetsByQuery(`-hidden -"secret:private"`, false)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"photo": 1}, facetsByKind(facets))

	// A missing exclusion is a maintained-summary/root-summary no-op.
	facets, err = client.KindFacetsByQuery("-does_not_exist", false)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"photo": 2, "video": 1}, facetsByKind(facets))
}
