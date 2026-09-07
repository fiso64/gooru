package gooru

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gooru.local/types"
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

	got, ok := parseSimpleUserTagFacetExclusions(`-"hidden" -"private:yes"`)
	if assert.True(t, ok) && assert.Len(t, got, 2) {
		assert.Equal(t, "hidden", got[0].Tag.Key)
		assert.True(t, got[0].KeyOnly)
		assert.Equal(t, "private", got[1].Tag.Key)
		assert.Equal(t, "yes", got[1].Tag.Value)
		assert.False(t, got[1].KeyOnly)
	}

	for _, expression := range []string{
		"hidden",
		"-hidden visible",
		"-hidden|-private",
		"-@tagged",
		"-type:photo",
		"-ext:jpg",
		"-hid*",
	} {
		_, ok := parseSimpleUserTagFacetExclusions(expression)
		assert.False(t, ok, expression)
	}
}

func TestSubtractKindFacets(t *testing.T) {
	t.Parallel()
	all := []types.TagWithCount{
		{Tag: "photo", Count: 8},
		{Tag: "video", Count: 4},
		{Tag: "comic", Count: 2},
	}
	excluded := []types.TagWithCount{
		{Tag: "photo", Count: 3},
		{Tag: "video", Count: 4},
	}
	assert.Equal(t, []types.TagWithCount{
		{Tag: "photo", Count: 5},
		{Tag: "comic", Count: 2},
	}, subtractKindFacets(all, excluded))
}
