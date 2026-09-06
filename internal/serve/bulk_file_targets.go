package serve

import "context"

// bulkFileTarget is the canonical server-side boundary for bulk-action selectors.
// Snapshot selectors are resolved here so bulk handlers cannot accidentally fall
// back to a live query after a snapshot expires or changes underneath them.
type bulkFileTarget struct {
	FileIDs        []string
	Query          string
	SelectionID    string
	IncludeFileIDs []string
	ExcludeFileIDs []string
}

func (s *Server) resolveBulkFileTarget(ctx context.Context, ownerID string, target bulkFileTarget) (bulkFileTarget, error) {
	if err := ctx.Err(); err != nil {
		return bulkFileTarget{}, err
	}
	if target.SelectionID == "" {
		return target, nil
	}
	fileIDs, err := s.fileSelections.resolve(ownerID, target.SelectionID)
	if err != nil {
		return bulkFileTarget{}, err
	}
	target.FileIDs = mergeSnapshotSelectionIDs(fileIDs, target.IncludeFileIDs, target.ExcludeFileIDs)
	target.Query = ""
	target.SelectionID = ""
	target.IncludeFileIDs = nil
	target.ExcludeFileIDs = nil
	return target, nil
}
