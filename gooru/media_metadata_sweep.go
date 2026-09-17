package gooru

import (
	"fmt"
	"strconv"
	"strings"

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
	mediaMetadataSweepCursorPrefix        = "after-location:"
	mediaMetadataRegistrationInputKey     = "registration"
)

func mediaMetadataRegistrationHook(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
	return mediaMetadataRegistrationTasksForOperation(len(event.ContentHashes) > 0, event.OperationID)
}

func mediaMetadataRegistrationTasks(hasRegistrations bool) ([]BackgroundTaskRequest, error) {
	return mediaMetadataRegistrationTasksForOperation(hasRegistrations, "")
}

func mediaMetadataRegistrationTasksForOperation(hasRegistrations bool, operationID string) ([]BackgroundTaskRequest, error) {
	if !hasRegistrations {
		return nil, nil
	}
	wake, err := newMediaMetadataSweepTaskRequest()
	if err != nil {
		return nil, err
	}
	operationID = strings.TrimSpace(operationID)
	if operationID != "" {
		wake.OperationID = operationID
	} else {
		wake.Operation = &BackgroundOperationRequest{
			Kind:    BackgroundMediaMetadataSweepOperationKind,
			Visible: true,
		}
		wake.OperationBinding = BackgroundOperationReuseActive
	}
	wake.CoalescePendingEquivalent = true
	wake.InputKey = mediaMetadataRegistrationInputKey
	return []BackgroundTaskRequest{wake}, nil
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

func mediaMetadataSweepContinuationTaskRequest(operationID string, afterLocationID int64) (BackgroundTaskRequest, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return BackgroundTaskRequest{}, fmt.Errorf("media metadata sweep continuation requires an operation id")
	}
	if afterLocationID <= 0 {
		return BackgroundTaskRequest{}, fmt.Errorf("media metadata sweep continuation cursor must be positive")
	}
	cursor := mediaMetadataSweepCursorPrefix + strconv.FormatInt(afterLocationID, 10)
	return BackgroundTaskRequest{
		OperationID:    operationID,
		DedupeKey:      BackgroundMediaMetadataSweepTaskKind + ":" + operationID + ":" + cursor,
		Kind:           BackgroundMediaMetadataSweepTaskKind,
		SubjectKind:    "library",
		SubjectID:      "media-metadata",
		InputKey:       cursor,
		ResourceClass:  BackgroundMediaMetadataResourceClass,
		MaxAttempts:    5,
	}, nil
}

// MediaMetadataSweepAfterLocationID decodes the durable keyset cursor carried by
// a continuation task. Initial/manual/registration wake tasks intentionally use
// opaque wake IDs and therefore start at zero. Unknown opaque values are kept as
// initial-task inputs for compatibility with already persisted sweep wakes.
func MediaMetadataSweepAfterLocationID(task BackgroundTask) (int64, error) {
	if !strings.HasPrefix(task.InputKey, mediaMetadataSweepCursorPrefix) {
		return 0, nil
	}
	raw := strings.TrimPrefix(task.InputKey, mediaMetadataSweepCursorPrefix)
	afterLocationID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || afterLocationID <= 0 {
		return 0, fmt.Errorf("invalid media metadata sweep cursor %q", task.InputKey)
	}
	return afterLocationID, nil
}

// EnqueueMediaMetadataSweepContinuation yields a bounded sweep quantum while
// keeping the next page attached to the same visible operation. The continuation
// uses the ordinary media resource and priority, so older queued media work can
// run before it. Active dedupe makes replay after a worker crash idempotent.
func (c *Client) EnqueueMediaMetadataSweepContinuation(operationID string, afterLocationID int64) (bool, error) {
	request, err := mediaMetadataSweepContinuationTaskRequest(operationID, afterLocationID)
	if err != nil {
		return false, err
	}
	_, created, err := c.EnqueueBackgroundTask(request)
	if err != nil {
		return false, fmt.Errorf("enqueue media metadata sweep continuation: %w", err)
	}
	return created, nil
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
