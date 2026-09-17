package gooru

// BackgroundOperationResultSummary is the compact completion projection used by
// history/list consumers that do not need the full structured operation result.
type BackgroundOperationResultSummary struct {
	Outcome       string
	AffectedCount *int64
	FailedCount   *int64
}

// GetBackgroundOperationResultSummaries returns any persisted compact summaries
// for the requested operation IDs. Operations without a summary are omitted.
func (c *Client) GetBackgroundOperationResultSummaries(operationIDs []string) (map[string]BackgroundOperationResultSummary, error) {
	stored, err := c.store.GetBackgroundOperationResultSummaries(operationIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[string]BackgroundOperationResultSummary, len(stored))
	for operationID, summary := range stored {
		result[operationID] = BackgroundOperationResultSummary{
			Outcome:       summary.Outcome,
			AffectedCount: summary.AffectedCount,
			FailedCount:   summary.FailedCount,
		}
	}
	return result, nil
}
