package gooru

import "gooru.local/types"

// KindFacetsForSimpleUserTagExcluding answers the WebUI's common shape of one
// positive user tag plus hidden-tag exclusions. The positive partition comes
// from maintained summaries while the excluded intersection is read live from
// indexed tag associations. ok is false for shapes that should stay on the
// general query path.
func (c *Client) KindFacetsForSimpleUserTagExcluding(expression string, exclusionTags []string) ([]types.TagWithCount, bool, error) {
	included, ok := parseSimpleUserTagFacetFilter(expression)
	if !ok {
		return nil, false, nil
	}

	exclusions := make([]tagFacetExclusion, 0, len(exclusionTags))
	for _, tag := range exclusionTags {
		filter, ok := parseSimpleUserTagFacetTerm(tag)
		if !ok {
			return nil, false, nil
		}
		exclusions = append(exclusions, tagFacetExclusion{Tag: filter.Tag, KeyOnly: filter.KeyOnly})
	}

	var (
		all []types.TagWithCount
		err error
	)
	if included.KeyOnly {
		all, err = c.store.KindFacetsForTagKey(included.Tag.Key)
	} else {
		all, err = c.store.KindFacetsForTag(included.Tag.Key, included.Tag.Value)
	}
	if err != nil || len(exclusions) == 0 {
		return all, true, err
	}

	excluded, err := c.store.KindFacetsForIncludedTagExcludingTags(
		tagFacetExclusion{Tag: included.Tag, KeyOnly: included.KeyOnly}, exclusions,
	)
	if err != nil {
		return nil, true, err
	}
	return subtractKindFacets(all, excluded), true, nil
}
