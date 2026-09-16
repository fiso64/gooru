from pathlib import Path

path = Path("gooru/tagging.go")
text = path.read_text()
start = text.index("// applyTaggingOperationInTx contains the switch logic")
end = text.index("\nfunc (c *Client) persistTaggingFollowUpInTx", start)
replacement = r'''// applyTaggingOperationInTx contains the switch logic for tag, settags, and untag.
func (c *Client) applyTaggingOperationInTx(tx *database.Tx, hashes []string, tags []string, kind opKind) (int64, error) {
	var affectedCount int64
	switch kind {
	case opTag:
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
			if err != nil {
				return 0, fmt.Errorf("failed to get or create tags: %w", err)
			}
			tagIDs := make([]int64, 0, len(tags))
			for _, tagStr := range tags {
				tagIDs = append(tagIDs, tagIDMap[tagStr])
			}
			affected, err := c.store.BatchAssociateTagsForContentHashes(tx, hashes, tagIDs)
			if err != nil {
				return 0, fmt.Errorf("failed to batch associate tags: %w", err)
			}
			affectedCount = affected
		}
	case opSetTags:
		cleared, err := c.store.BatchClearTagsForContent(tx, hashes)
		if err != nil {
			return 0, fmt.Errorf("failed to batch clear tags: %w", err)
		}
		var associated int64
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetOrCreateTags(tx, parsedTags)
			if err != nil {
				return 0, fmt.Errorf("failed to get or create tags: %w", err)
			}
			tagIDs := make([]int64, 0, len(tags))
			for _, tagStr := range tags {
				tagIDs = append(tagIDs, tagIDMap[tagStr])
			}
			associated, err = c.store.BatchAssociateTagsForContentHashes(tx, hashes, tagIDs)
			if err != nil {
				return 0, fmt.Errorf("failed to batch associate tags: %w", err)
			}
		}
		affectedCount = cleared + associated
	case opUntag:
		if len(tags) > 0 {
			parsedTags := make([]types.ParsedTag, len(tags))
			for i, t := range tags {
				parsedTags[i] = query.ParseTag(t)
			}
			tagIDMap, err := c.store.BatchGetTags(tx, parsedTags)
			if err != nil {
				return 0, fmt.Errorf("failed to look up tags: %w", err)
			}
			tagIDs := make([]int64, 0, len(tags))
			for _, tagStr := range tags {
				if tagID, ok := tagIDMap[tagStr]; ok {
					tagIDs = append(tagIDs, tagID)
				}
			}
			affected, err := c.store.BatchDisassociateTagsForContentHashes(tx, hashes, tagIDs)
			if err != nil {
				return 0, fmt.Errorf("failed to batch disassociate tags: %w", err)
			}
			affectedCount = affected
		}
	}
	return affectedCount, nil
}
'''
path.write_text(text[:start] + replacement + text[end:])
