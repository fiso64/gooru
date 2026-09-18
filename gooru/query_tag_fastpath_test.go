package gooru

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimpleUserTagFacetFilter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		query   string
		key     string
		value   string
		keyOnly bool
		ok      bool
	}{
		{query: "hidden", key: "hidden", keyOnly: true, ok: true},
		{query: "artist:Alice", key: "artist", value: "Alice", ok: true},
		{query: "artist:", key: "artist", ok: true},
		{query: " hidden ", key: "hidden", keyOnly: true, ok: true},
		{query: "hidden other", ok: false},
		{query: "hidden|other", ok: false},
		{query: "-hidden", ok: false},
		{query: "@tagged", ok: false},
		{query: "ext:jpg", ok: false},
		{query: "type:photo", ok: false},
		{query: "hid*", ok: false},
	} {
		tc := tc
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			got, ok := parseSimpleUserTagFacetFilter(tc.query)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.key, got.Tag.Key)
			assert.Equal(t, tc.value, got.Tag.Value)
			assert.Equal(t, tc.keyOnly, got.KeyOnly)
		})
	}
}

func TestSimpleUserTagFacetExclusions(t *testing.T) {
	t.Parallel()
	filters, ok := parseSimpleUserTagFacetExclusions(`-hidden -"secret:private"`)
	require.True(t, ok)
	require.Len(t, filters, 2)
	assert.Equal(t, "hidden", filters[0].Tag.Key)
	assert.True(t, filters[0].KeyOnly)
	assert.Equal(t, "secret", filters[1].Tag.Key)
	assert.Equal(t, "private", filters[1].Tag.Value)
	assert.False(t, filters[1].KeyOnly)

	for _, query := range []string{"hidden", "-hidden visible", "-hidden | -secret", "-@tagged", "-hid*", "-(hidden | secret)"} {
		_, ok := parseSimpleUserTagFacetExclusions(query)
		assert.False(t, ok, query)
	}
}


func TestSimpleExactUserTag(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		query string
		key   string
		value string
		ok    bool
	}{
		{query: "artist:Alice", key: "artist", value: "Alice", ok: true},
		{query: "artist:", key: "artist", ok: true},
		{query: "hidden", ok: false},
		{query: "@tagged", ok: false},
		{query: "ext:jpg", ok: false},
		{query: "type:photo", ok: false},
		{query: "-artist:Alice", ok: false},
		{query: "artist:Alice|artist:Bob", ok: false},
	} {
		tc := tc
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			ast, err := parseAndValidateQuery(tc.query)
			require.NoError(t, err)
			tag, ok := simpleExactUserTag(ast)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.key, tag.Key)
			assert.Equal(t, tc.value, tag.Value)
		})
	}
}
