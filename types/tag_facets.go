package types

// TagFacetExclusion describes one simple user-tag exclusion for aggregate
// queries. KeyOnly matches every value in the namespace; otherwise Tag is an
// exact key/value match.
type TagFacetExclusion struct {
	Tag     ParsedTag
	KeyOnly bool
}
