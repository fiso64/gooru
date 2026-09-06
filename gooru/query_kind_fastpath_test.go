package gooru

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gooru.local/types"
)

func TestSimpleKindFacetFilter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		query string
		kind  string
		ok    bool
	}{
		{query: "type:photo", kind: "photo", ok: true},
		{query: " type:VIDEO ", kind: "video", ok: true},
		{query: "type:comic", kind: "comic", ok: true},
		{query: "type:img", ok: false},
		{query: "type:photo type:video", ok: false},
		{query: "type:photo|type:video", ok: false},
		{query: "-type:photo", ok: false},
		{query: "type:pho*", ok: false},
		{query: "ext:jpg", ok: false},
	} {
		tc := tc
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			kind, ok := simpleKindFacetFilter(tc.query)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.kind, kind)
		})
	}
}

func TestFilterKindFacet(t *testing.T) {
	t.Parallel()
	facets := []types.TagWithCount{{Tag: "photo", Count: 8}, {Tag: "video", Count: 3}}

	assert.Equal(t, []types.TagWithCount{{Tag: "video", Count: 3}}, filterKindFacet(facets, "video"))
	empty := filterKindFacet(facets, "audio")
	require.NotNil(t, empty)
	assert.Empty(t, empty)
}
