package gooru

import (
	"sort"

	"gooru.local/types"
)

func subtractKindFacets(all, excluded []types.TagWithCount) []types.TagWithCount {
	if len(all) == 0 {
		return nil
	}
	excludedCounts := make(map[string]int, len(excluded))
	for _, facet := range excluded {
		excludedCounts[facet.Tag] += facet.Count
	}
	out := make([]types.TagWithCount, 0, len(all))
	for _, facet := range all {
		count := facet.Count - excludedCounts[facet.Tag]
		if count <= 0 {
			continue
		}
		out = append(out, types.TagWithCount{Tag: facet.Tag, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out
}

// kindFacetsForUserTagExclusions answers a conjunction made only of negative
// simple user tags. One exclusion stays entirely on maintained summaries. For
// multiple exclusions we still use the maintained root summary, while the
// overlap-sensitive excluded union is aggregated from indexed tag associations
// so work scales with the excluded set rather than the full included library.
func (c *Client) kindFacetsForUserTagExclusions(filters []simpleUserTagFacetFilter) ([]types.TagWithCount, error) {
	all, err := c.store.KindFacets()
	if err != nil {
		return nil, err
	}
	if len(filters) == 0 {
		return all, nil
	}

	var excluded []types.TagWithCount
	if len(filters) == 1 {
		filter := filters[0]
		if filter.KeyOnly {
			excluded, err = c.store.KindFacetsForTagKey(filter.Tag.Key)
		} else {
			excluded, err = c.store.KindFacetsForTag(filter.Tag.Key, filter.Tag.Value)
		}
	} else {
		exclusions := make([]tagFacetExclusion, 0, len(filters))
		for _, filter := range filters {
			exclusions = append(exclusions, tagFacetExclusion{Tag: filter.Tag, KeyOnly: filter.KeyOnly})
		}
		excluded, err = c.store.KindFacetsForExcludedTags(exclusions)
	}
	if err != nil {
		return nil, err
	}
	return subtractKindFacets(all, excluded), nil
}
