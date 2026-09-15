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
	return mediaMetadataRegistrationTasks(len(event.ContentHashes) > 0)
}

func mediaMetadataRegistrationTasks(hasRegistrations bool) ([]BackgroundTaskRequest, error) {
	if !hasRegistrations {
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
	task.OperationBinding = BackgroundOperationReuseActive
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

// MediaMetadataSweepRunning reports whether a metadata sweep operation is
// currently pending or running. The operation kind is the durable single-flight
// identity shared by registration-triggered, recovery, and manual runs.
func (c *Client) MediaMetadataSweepRunning() (bool, error) {
	_, found, err := c.store.FindActiveBackgroundOperationIDByKind(c.store.DB, BackgroundMediaMetadataSweepOperationKind)
	if err != nil {
		return false, fmt.Errorf("inspect active media metadata sweep: %w", err)
	}
	return found, nil
}

// RunMediaMetadataSweep starts one visible metadata sweep whenever one is not
// already active. Unlike startup recovery, an explicit manual run creates an
// observable job even when there is currently no pending metadata; the worker
// will scan, find no work, and complete the operation normally.
func (c *Client) RunMediaMetadataSweep() (bool, error) {
	task, err := newMediaMetadataSweepTaskRequest()
	if err != nil {
		return false, err
	}
	_, tasks, created, err := c.createBackgroundOperationWithTasksSingleFlight(BackgroundOperationRequest{
		Kind:    BackgroundMediaMetadataSweepOperationKind,
		Visible: true,
	}, []BackgroundTaskRequest{task})
	if err != nil {
		return false, fmt.Errorf("enqueue media metadata sweep: %w", err)
	}
	if !created {
		return false, nil
	}
	if len(tasks) != 1 {
		return false, fmt.Errorf("enqueue media metadata sweep created %d tasks, want 1", len(tasks))
	}
	return true, nil
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
	return c.RunMediaMetadataSweep()
}

func (c *Client) ListPendingMediaMetadataFiles(afterLocationID int64, limit int) ([]types.FileInfo, error) {
	return c.store.ListPendingMediaMetadataFiles(afterLocationID, limit)
}

func (c *Client) UpsertMediaMetadataForLocation(locationID int64, expectedHash, expectedPath string, meta types.MediaMetadata) (bool, error) {
	return c.store.UpsertMediaMetadataForLocation(locationID, expectedHash, expectedPath, meta)
}
