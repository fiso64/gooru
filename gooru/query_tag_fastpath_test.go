package gooru

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
