package database

// BatchAssociateTagsForContentHashes associates every content hash with every
// tag ID while bounding transient pair storage to one SQLite-sized batch.
func (s *Store) BatchAssociateTagsForContentHashes(q Querier, hashes []string, tagIDs []int64) (int64, error) {
	return s.batchContentTagCartesian(q, hashes, tagIDs, s.BatchAssociateTags)
}

// BatchDisassociateTagsForContentHashes removes every requested content/tag
// association while bounding transient pair storage to one SQLite-sized batch.
func (s *Store) BatchDisassociateTagsForContentHashes(q Querier, hashes []string, tagIDs []int64) (int64, error) {
	return s.batchContentTagCartesian(q, hashes, tagIDs, s.BatchDisassociateTags)
}

func (s *Store) batchContentTagCartesian(
	q Querier,
	hashes []string,
	tagIDs []int64,
	apply func(Querier, []ContentTagPair) (int64, error),
) (int64, error) {
	if len(hashes) == 0 || len(tagIDs) == 0 {
		return 0, nil
	}

	const columns = 2
	batchSize := maxVars / columns
	pairs := make([]ContentTagPair, 0, batchSize)
	var totalAffected int64

	flush := func() error {
		affected, err := apply(q, pairs)
		if err != nil {
			return err
		}
		totalAffected += affected
		pairs = pairs[:0]
		return nil
	}

	for _, hash := range hashes {
		for _, tagID := range tagIDs {
			pairs = append(pairs, ContentTagPair{ContentHash: hash, TagID: tagID})
			if len(pairs) == batchSize {
				if err := flush(); err != nil {
					return 0, err
				}
			}
		}
	}
	if len(pairs) > 0 {
		if err := flush(); err != nil {
			return 0, err
		}
	}
	return totalAffected, nil
}
