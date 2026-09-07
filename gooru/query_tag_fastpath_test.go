package gooru

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleExactUserTagFilter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		query string
		key   string
		value string
		ok    bool
	}{
		{query: "hidden", key: "hidden", ok: true},
		{query: "artist:Alice", key: "artist", value: "Alice", ok: true},
		{query: "artist:", key: "artist", ok: true},
		{query: " hidden ", key: "hidden", ok: true},
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
			got, ok := simpleExactUserTagFilter(tc.query)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.key, got.Key)
			assert.Equal(t, tc.value, got.Value)
		})
	}
}
