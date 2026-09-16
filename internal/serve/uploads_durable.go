package serve

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
)

const (
	backgroundUploadCleanupTaskKind         = "upload.cleanup"
	backgroundUploadCleanupPriority         = 100
	backgroundUploadWaitInitialPollInterval = 25 * time.Millisecond
	backgroundUploadWaitMaximumPollInterval = 250 * time.Millisecond
	uploadReservationHeader                 = "X-Gooru-Upload-Reserve"
	uploadSegmentCountHeader                = "X-Gooru-Upload-Segment-Count"
	uploadOperationHeader                   = "X-Gooru-Upload-Operation-ID"
)

// durableUploadOperationStore is the producer/read boundary required by HTTP
// uploads. Admission starts hidden, then the receiving checkpoint is published
// before multipart staging so the same operation is visible throughout transport
// and durable import.
type durableUploadOperationStore interface {
	backgroundOperationReader
	CreateBackgroundOperation(core.BackgroundOperationRequest) (core.BackgroundOperation, error)
	SetBackgroundOperationCheckpoint(string, any) error
	SetBackgroundOperationVisible(string, bool) error
	ClaimBackgroundOperationProducer(string) (bool, error)
	AttachBackgroundTaskAndRevealOperation(string, any, core.BackgroundTaskRequest) (core.BackgroundTask, error)
}

type durableUploadCancellationStore interface {
	CancelBackgroundOperationWithCleanupTask(string, core.BackgroundTaskRequest) (core.BackgroundOperationCancellation, error)
}

type durableUploadTaskReader interface {
	GetBackgroundOperationTask(string) (core.BackgroundTaskState, bool, error)
}

type durableUploadCleanupStore interface {
	durableUploadTaskReader
	GetBackgroundOperationCheckpoint(string, any) (bool, error)
}

func (l *GooruLibrary) CreateBackgroundOperation(request core.BackgroundOperationRequest) (core.BackgroundOperation, error) {
	return l.client.CreateBackgroundOperation(request)
}

func (l *GooruLibrary) ClaimBackgroundOperationProducer(operationID string) (bool, error) {
	return l.client.ClaimBackgroundOperationProducer(operationID)
}

func (l *GooruLibrary) AttachBackgroundTaskAndRevealOperation(operationID string, checkpoint any, request core.BackgroundTaskRequest) (core.BackgroundTask, error) {
	return l.client.AttachBackgroundTaskAndRevealOperation(operationID, checkpoint, request)
}

func (l *GooruLibrary) GetBackgroundOperationTask(operationID string) (core.BackgroundTaskState, bool, error) {
	return l.client.GetBackgroundOperationTask(operationID)
}

func (l *GooruLibrary) CancelBackgroundOperationWithCleanupTask(operationID string, request core.BackgroundTaskRequest) (core.BackgroundOperationCancellation, error) {
	return l.client.CancelBackgroundOperationWithCleanupTask(operationID, request)
}

func (s *Server) handleUploadEndpoint(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.backgroundOperations.(durableUploadOperationStore); ok {
		s.handleDurableUpload(w, r)
		return
	}
	s.handleUpload(w, r)
}

func (s *Server) handleDurableUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "upload import service is not configured", nil)
		return
	}
	operations, ok := s.backgroundOperations.(durableUploadOperationStore)
	if !ok || operations == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation service is not configured", nil)
		return
	}
	if !s.cfg.Uploads.Enabled {
		writeError(w, http.StatusForbidden, "uploads_disabled", "uploads are disabled", nil)
		return
	}
	if !hasUploadTarget(s.cfg.Uploads.Targets) {
		writeError(w, http.StatusForbidden, "uploads_disabled", "upload target is not configured", nil)
		return
	}

	if strings.EqualFold(strings.TrimSpace(r.Header.Get(uploadReservationHeader)), "true") {
		segmentCount, err := durableUploadReservationSegmentCount(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		s.reserveDurableUpload(w, operations, segmentCount)
		return
	}

	reservationID := strings.TrimSpace(r.Header.Get(uploadOperationHeader))
	admission, err := s.admitDurableUpload(r, operations)
	if err != nil {
		if reservationID == "" && len(r.Header.Values(uploadSegmentIndexHeader)) == 0 {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to accept upload", nil)
		} else {
			writeError(w, http.StatusConflict, "invalid_upload_reservation", err.Error(), nil)
		}
		return
	}
	operation := admission.operation
	if admission.alreadyAttached {
		if err := writeDurableUploadAccepted(w, s, operations, operation.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load upload operation", nil)
		}
		return
	}

	attached := false
	defer func() {
		if !attached {
			failDurableUploadProducer(operations, operation.ID)
		}
	}()

	if err := operations.SetBackgroundOperationCheckpoint(operation.ID, backgroundUploadReceivingCheckpoint(r.ContentLength, 0)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to initialize upload progress", nil)
		return
	}
	if admission.created {
		if err := operations.SetBackgroundOperationVisible(operation.ID, true); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to publish upload operation", nil)
			return
		}
	}
	r.Body = newUploadReceivingProgressReadCloser(r.Body, operation.ID, r.ContentLength, operations)

	tags, saved, err := s.stageDurableMultipartUpload(r, operation.ID)
	if err != nil {
		if errors.Is(err, errUploadReceivingCanceled) {
			writeError(w, http.StatusRequestTimeout, "request_canceled", "upload was canceled", nil)
			return
		}
		writeMultipartUploadError(w, err)
		return
	}
	if err := query.ValidateTags(tags); err != nil {
		removeSavedUploads(saved)
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if len(saved) == 1 && saved[0].status == "error" && saved[0].error == errUploadTooLarge.Error() {
		fileErr := uploadFileError{name: saved[0].name, err: errUploadTooLarge}
		failDurableUploadProducerWithError(operations, operation.ID, "payload_too_large", fileErr.Error())
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
		return
	}

	taskRequest, err := backgroundUploadTaskRequest(operation.ID, saved, tags)
	if err != nil {
		removeSavedUploads(saved)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare uploaded files", nil)
		return
	}
	if admission.segmented {
		segmentStore := any(operations).(durableUploadSegmentStore)
		if _, _, err := segmentStore.AttachBackgroundTaskToOperation(operation.ID, admission.taskID, backgroundUploadInitialCheckpoint(len(saved)), taskRequest); err != nil {
			removeSavedUploads(saved)
			_ = cleanupDurableUploadStagingDirs(saved)
			if state, found, stateErr := operations.GetBackgroundOperation(operation.ID); stateErr == nil && found && state.Status == core.BackgroundWorkCanceled {
				writeError(w, http.StatusRequestTimeout, "request_canceled", "upload was canceled", nil)
				return
			}
			if existing, found, readErr := segmentStore.GetBackgroundTask(admission.taskID); readErr == nil && found && durableUploadSegmentTaskMatches(existing, operation.ID) {
				attached = true
				if err := writeDurableUploadAccepted(w, s, operations, operation.ID); err != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", "failed to load upload operation", nil)
				}
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to queue uploaded files", nil)
			return
		}
	} else {
		if _, err := operations.AttachBackgroundTaskAndRevealOperation(operation.ID, backgroundUploadInitialCheckpoint(len(saved)), taskRequest); err != nil {
			removeSavedUploads(saved)
			if state, found, stateErr := operations.GetBackgroundOperation(operation.ID); stateErr == nil && found && state.Status == core.BackgroundWorkCanceled {
				writeError(w, http.StatusRequestTimeout, "request_canceled", "upload was canceled", nil)
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to queue uploaded files", nil)
			return
		}
	}
	attached = true

	if PreferAsync(r) {
		if err := writeDurableUploadAccepted(w, s, operations, operation.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load upload operation", nil)
		}
		return
	}

	response, err := waitForDurableUpload(r.Context(), operations, operation.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			if _, cancelErr := s.cancelBackgroundOperation(operation.ID); cancelErr != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to cancel upload operation", nil)
				return
			}
			writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func durableUploadReservationSegmentCount(r *http.Request) (int64, error) {
	values := r.Header.Values(uploadSegmentCountHeader)
	if len(values) == 0 {
		return 1, nil
	}
	if len(values) != 1 {
		return 0, errors.New("upload segment count must be one positive integer")
	}
	value := strings.TrimSpace(values[0])
	if value == "" {
		return 0, errors.New("upload segment count must be a positive integer")
	}
	segmentCount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || segmentCount <= 0 {
		return 0, errors.New("upload segment count must be a positive integer")
	}
	return segmentCount, nil
}

func (s *Server) reserveDurableUpload(w http.ResponseWriter, operations durableUploadOperationStore, segmentCount int64) {
	operation, err := operations.CreateBackgroundOperation(core.BackgroundOperationRequest{Kind: backgroundUploadImportOperationKind, Visible: false, ProgressTotal: segmentCount})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to reserve upload", nil)
		return
	}
	committed := false
	defer func() {
		if !committed {
			failDurableUploadProducer(operations, operation.ID)
		}
	}()
	if err := operations.SetBackgroundOperationCheckpoint(operation.ID, backgroundUploadReceivingCheckpoint(0, 0)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to initialize upload reservation", nil)
		return
	}
	if err := operations.SetBackgroundOperationVisible(operation.ID, true); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to publish upload reservation", nil)
		return
	}
	state, found, err := operations.GetBackgroundOperation(operation.ID)
	if err != nil || !found {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load upload reservation", nil)
		return
	}
	committed = true
	w.Header().Set("Location", "/api/v1/operations/"+operation.ID)
	writeJSON(w, http.StatusCreated, s.backgroundOperationDTO(state))
}

func (s *Server) claimDurableUploadOperation(r *http.Request, operations durableUploadOperationStore) (core.BackgroundOperation, bool, error) {
	operationID := strings.TrimSpace(r.Header.Get(uploadOperationHeader))
	if operationID == "" {
		operation, err := operations.CreateBackgroundOperation(core.BackgroundOperationRequest{Kind: backgroundUploadImportOperationKind, Visible: false, ProgressTotal: 1})
		return operation, true, err
	}
	if !validDurableUploadOperationID(operationID) {
		return core.BackgroundOperation{}, false, errors.New("upload reservation is invalid")
	}
	state, found, err := operations.GetBackgroundOperation(operationID)
	if err != nil {
		return core.BackgroundOperation{}, false, err
	}
	if !found || state.Kind != backgroundUploadImportOperationKind || !state.Visible {
		return core.BackgroundOperation{}, false, errors.New("upload reservation was not found")
	}
	if state.Status != core.BackgroundWorkPending {
		return core.BackgroundOperation{}, false, errors.New("upload reservation is not active")
	}
	claimed, err := operations.ClaimBackgroundOperationProducer(operationID)
	if err != nil {
		return core.BackgroundOperation{}, false, err
	}
	if !claimed {
		return core.BackgroundOperation{}, false, errors.New("upload reservation was already claimed")
	}
	return core.BackgroundOperation{ID: state.ID, Kind: state.Kind, Visible: state.Visible, ProgressTotal: state.ProgressTotal, CreatedAt: state.CreatedAt}, false, nil
}

func (s *Server) cancelBackgroundOperation(operationID string) (bool, error) {
	if s.backgroundOperations == nil {
		return false, errors.New("background operation service is not configured")
	}
	state, found, err := s.backgroundOperations.GetBackgroundOperation(operationID)
	if err != nil {
		return false, err
	}
	if !found || state.Kind != backgroundUploadImportOperationKind {
		return s.backgroundOperations.CancelBackgroundOperation(operationID)
	}
	if taskStore, ok := s.backgroundOperations.(durableUploadTaskReader); ok {
		if _, found, taskErr := taskStore.GetBackgroundOperationTask(operationID); taskErr != nil {
			return false, taskErr
		} else if !found {
			return s.backgroundOperations.CancelBackgroundOperation(operationID)
		}
	}
	store, ok := s.backgroundOperations.(durableUploadCancellationStore)
	if !ok {
		return s.backgroundOperations.CancelBackgroundOperation(operationID)
	}
	result, err := store.CancelBackgroundOperationWithCleanupTask(operationID, backgroundUploadCleanupTaskRequest(operationID))
	return result.Canceled, err
}

func backgroundUploadCleanupTaskRequest(operationID string) core.BackgroundTaskRequest {
	return core.BackgroundTaskRequest{DedupeKey: "upload-cleanup:" + operationID, Kind: backgroundUploadCleanupTaskKind, SubjectKind: "operation", SubjectID: operationID, ResourceClass: backgroundUploadResourceClass, Priority: backgroundUploadCleanupPriority, MaxAttempts: 5}
}

func (s *Server) backgroundUploadCleanupHandler(store durableUploadCleanupStore) core.BackgroundTaskHandler {
	return func(ctx context.Context, task core.BackgroundTask) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if task.OperationID != "" || task.Kind != backgroundUploadCleanupTaskKind || task.SubjectKind != "operation" || task.SubjectID == "" {
			return errors.New("upload cleanup task has invalid operation identity")
		}
		original, found, err := store.GetBackgroundOperationTask(task.SubjectID)
		if err != nil {
			return fmt.Errorf("load canceled upload task: %w", err)
		}
		if !found {
			return errors.New("canceled upload task is missing")
		}
		return cleanupCanceledDurableUpload(store, task.SubjectID, original.BackgroundTask)
	}
}

func cleanupCanceledDurableUpload(store durableUploadCleanupStore, operationID string, task core.BackgroundTask) error {
	files, _, err := decodeBackgroundUploadTask(task)
	if err != nil {
		return fmt.Errorf("decode canceled background upload task: %w", err)
	}
	var checkpoint backgroundUploadCheckpoint
	found, err := store.GetBackgroundOperationCheckpoint(operationID, &checkpoint)
	if err != nil {
		return fmt.Errorf("load canceled upload checkpoint: %w", err)
	}
	if !found {
		return errors.New("canceled upload checkpoint is missing")
	}
	switch checkpoint.Phase {
	case backgroundUploadPhaseStaged:
		return removeCanceledSavedUploads(files)
	case backgroundUploadPhaseActivated:
		return cleanupCanceledClaimedUpload(files)
	case backgroundUploadPhaseImported:
		if checkpoint.Response == nil {
			return errors.New("canceled imported upload checkpoint is missing response")
		}
		if err := settleDurableNonreplacementActivations(files); err != nil {
			return fmt.Errorf("settle canceled imported durable uploads: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("canceled upload checkpoint has invalid phase %q", checkpoint.Phase)
	}
}

func removeCanceledSavedUploads(files []savedUpload) error {
	failures := 0
	for _, file := range files {
		if file.status == "skipped" || file.status == "error" {
			continue
		}
		if err := os.Remove(file.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("remove %d canceled staged upload files", failures)
	}
	if err := cleanupDurableUploadStagingDirs(files); err != nil {
		return fmt.Errorf("prune canceled staged upload directories: %w", err)
	}
	return nil
}

func cleanupDurableUploadStagingDirs(files []savedUpload) error {
	for _, file := range files {
		if file.status == "skipped" || file.status == "error" || file.path == "" {
			continue
		}

		fileDir := filepath.Clean(filepath.Dir(file.path))
		operationDir := fileDir
		if strings.HasPrefix(filepath.Base(fileDir), "request-") {
			operationDir = filepath.Dir(fileDir)
		}
		if !validDurableUploadOperationID(filepath.Base(operationDir)) {
			continue
		}
		stagingRoot := filepath.Dir(operationDir)
		if filepath.Base(stagingRoot) != durableUploadStagingRootName {
			continue
		}

		if filepath.Clean(fileDir) != filepath.Clean(operationDir) {
			if err := removeEmptyDurableUploadStagingDir(fileDir); err != nil {
				return err
			}
		}
		if err := removeEmptyDurableUploadStagingDir(operationDir); err != nil {
			return err
		}
		if err := removeEmptyDurableUploadStagingDir(stagingRoot); err != nil {
			return err
		}
	}
	return nil
}

func removeEmptyDurableUploadStagingDir(path string) error {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return nil
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		entries, readErr := os.ReadDir(path)
		if readErr == nil && len(entries) != 0 {
			return nil
		}
		return err
	}
	return nil
}

func waitForDurableUpload(ctx context.Context, operations backgroundOperationReader, operationID string) (UploadImportResponse, error) {
	pollInterval := backgroundUploadWaitInitialPollInterval
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		state, found, err := operations.GetBackgroundOperation(operationID)
		if err != nil {
			return UploadImportResponse{}, err
		}
		if !found {
			return UploadImportResponse{}, errors.New("upload operation disappeared")
		}
		switch state.Status {
		case core.BackgroundWorkCompleted:
			var response UploadImportResponse
			found, err := operations.GetBackgroundOperationResult(operationID, &response)
			if err != nil {
				return UploadImportResponse{}, err
			}
			if !found {
				return UploadImportResponse{}, errors.New("completed upload operation has no result")
			}
			return response, nil
		case core.BackgroundWorkFailed:
			if state.ErrorMessage != "" {
				return UploadImportResponse{}, errors.New(state.ErrorMessage)
			}
			return UploadImportResponse{}, errors.New("upload operation failed")
		case core.BackgroundWorkCanceled:
			return UploadImportResponse{}, context.Canceled
		}
		select {
		case <-ctx.Done():
			return UploadImportResponse{}, ctx.Err()
		case <-ticker.C:
			pollInterval = nextBackgroundUploadWaitPollInterval(pollInterval)
			ticker.Reset(pollInterval)
		}
	}
}

func nextBackgroundUploadWaitPollInterval(current time.Duration) time.Duration {
	next := current * 2
	if next > backgroundUploadWaitMaximumPollInterval {
		return backgroundUploadWaitMaximumPollInterval
	}
	return next
}
