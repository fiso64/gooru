package gooru

import (
	"fmt"

	"gooru.local/types"
)

const (
	// BackgroundMediaMetadataSweepOperationKind is the user-visible durable job
	// created for registration-triggered metadata sweeps.
	BackgroundMediaMetadataSweepOperationKind = "media.metadata-sweep"
	// BackgroundMediaMetadataSweepTaskKind is handled by the server media worker.
	BackgroundMediaMetadataSweepTaskKind = "media.metadata-sweep"
	// BackgroundMediaMetadataResourceClass serializes metadata extraction with
	// other media work such as thumbnail generation.
	BackgroundMediaMetadataResourceClass = "media"
)

func mediaMetadataRegistrationHook(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
	if len(event.ContentHashes) == 0 {
		return nil, nil
	}
	task, err := newMediaMetadataSweepTaskRequest()
	if err != nil {
		return nil, err
	}
	task.Operation = &BackgroundOperationRequest{
		Kind:    BackgroundMediaMetadataSweepOperationKind,
		Visible: true,
	}
	// Registration transactions use SQLite immediate locking, so lookup/create
	// of the active operation is serialized across independent CLI/server clients.
	// Each registration still gets a distinct child wake to avoid losing work if
	// it commits while an earlier sweep is finishing its final scan.
	task.reuseActiveOperation = true
	return []BackgroundTaskRequest{task}, nil
}

func newMediaMetadataSweepTaskRequest() (BackgroundTaskRequest, error) {
	wakeID, err := newBackgroundWorkID("media-metadata-wake")
	if err != nil {
		return BackgroundTaskRequest{}, fmt.Errorf("build media metadata sweep wake: %w", err)
	}
	return BackgroundTaskRequest{
		DedupeKey:     "media-metadata-sweep:" + wakeID,
		Kind:          BackgroundMediaMetadataSweepTaskKind,
		SubjectKind:   "library",
		SubjectID:     "media-metadata",
		InputKey:      wakeID,
		ResourceClass: BackgroundMediaMetadataResourceClass,
		MaxAttempts:   5,
	}, nil
}

// EnsureMediaMetadataSweep schedules one visible sweep when metadata is pending
// and no sweep operation is already pending or running. It is intended for
// server startup recovery so registrations created by older/broken producers are
// repaired after restart without stacking duplicate recovery jobs.
func (c *Client) EnsureMediaMetadataSweep() (bool, error) {
	pending, err := c.ListPendingMediaMetadataFiles(0, 1)
	if err != nil {
		return false, fmt.Errorf("inspect pending media metadata: %w", err)
	}
	if len(pending) == 0 {
		return false, nil
	}

	activeIDs, err := c.ListActiveBackgroundOperationIDs(false)
	if err != nil {
		return false, fmt.Errorf("list active background operations: %w", err)
	}
	for _, operationID := range activeIDs {
		operation, found, err := c.GetBackgroundOperation(operationID)
		if err != nil {
			return false, fmt.Errorf("inspect active background operation %s: %w", operationID, err)
		}
		if found && operation.Kind == BackgroundMediaMetadataSweepOperationKind {
			return false, nil
		}
	}

	task, err := newMediaMetadataSweepTaskRequest()
	if err != nil {
		return false, err
	}
	_, tasks, err := c.CreateBackgroundOperationWithTasks(BackgroundOperationRequest{
		Kind:    BackgroundMediaMetadataSweepOperationKind,
		Visible: true,
	}, []BackgroundTaskRequest{task})
	if err != nil {
		return false, fmt.Errorf("enqueue media metadata recovery sweep: %w", err)
	}
	if len(tasks) != 1 {
		return false, fmt.Errorf("enqueue media metadata recovery sweep created %d tasks, want 1", len(tasks))
	}
	return true, nil
}

func (c *Client) ListPendingMediaMetadataFiles(afterLocationID int64, limit int) ([]types.FileInfo, error) {
	return c.store.ListPendingMediaMetadataFiles(afterLocationID, limit)
}

func (c *Client) UpsertMediaMetadataForLocation(locationID int64, expectedHash, expectedPath string, meta types.MediaMetadata) (bool, error) {
	return c.store.UpsertMediaMetadataForLocation(locationID, expectedHash, expectedPath, meta)
}
