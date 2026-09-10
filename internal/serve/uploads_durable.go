package serve

import (
	"context"
	"errors"
	"net/http"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
)

const defaultDurableUploadPendingLimit = 64

// durableUploadOperationStore is the producer/read boundary required by HTTP
// upload admission. The operation is created hidden before multipart staging,
// then the recovery checkpoint, child task, and visibility transition are
// committed atomically once staging succeeds.
type durableUploadOperationStore interface {
	backgroundOperationReader
	CreateBackgroundOperationWithPendingLimit(core.BackgroundOperationRequest, int) (core.BackgroundOperation, error)
	AttachBackgroundTaskAndRevealOperation(string, any, core.BackgroundTaskRequest) (core.BackgroundTask, error)
}

func (l *GooruLibrary) AttachBackgroundTaskAndRevealOperation(operationID string, checkpoint any, request core.BackgroundTaskRequest) (core.BackgroundTask, error) {
	return l.client.AttachBackgroundTaskAndRevealOperation(operationID, checkpoint, request)
}

// handleUploadEndpoint uses the durable operation path whenever the configured
// library exposes the durable producer boundary. Importer-only Library test
// doubles and embedders without that boundary retain the legacy path until they
// opt into durable operations.
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

	operation, err := operations.CreateBackgroundOperationWithPendingLimit(core.BackgroundOperationRequest{
		Kind:          "upload_import",
		Visible:       false,
		ProgressTotal: 1,
	}, s.durableUploadPendingLimit())
	if err != nil {
		writeDurableUploadAdmissionError(w, err)
		return
	}
	attached := false
	defer func() {
		if !attached {
			_, _ = operations.CancelBackgroundOperation(operation.ID)
		}
	}()

	if limit := s.uploadRequestBodyLimit(); limit > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
	}
	tags, saved, err := s.stageMultipartUpload(r)
	if err != nil {
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
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
		return
	}

	taskRequest, err := backgroundUploadTaskRequest(operation.ID, saved, tags)
	if err != nil {
		removeSavedUploads(saved)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare uploaded files", nil)
		return
	}
	if _, err := operations.AttachBackgroundTaskAndRevealOperation(operation.ID, backgroundUploadInitialCheckpoint(), taskRequest); err != nil {
		removeSavedUploads(saved)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to queue uploaded files", nil)
		return
	}
	attached = true

	if PreferAsync(r) {
		state, found, err := operations.GetBackgroundOperation(operation.ID)
		if err != nil || !found {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load upload operation", nil)
			return
		}
		writeJSON(w, http.StatusAccepted, backgroundOperationDTO(state))
		return
	}

	response, err := waitForDurableUpload(r.Context(), operations, operation.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_, _ = operations.CancelBackgroundOperation(operation.ID)
			writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) durableUploadPendingLimit() int {
	if s.cfg.Jobs.MaxQueued > 0 {
		return s.cfg.Jobs.MaxQueued
	}
	return defaultDurableUploadPendingLimit
}

func writeDurableUploadAdmissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrBackgroundOperationPendingLimit):
		writeError(w, http.StatusServiceUnavailable, "job_queue_full", "job queue is full", nil)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to accept upload", nil)
	}
}

func waitForDurableUpload(ctx context.Context, operations backgroundOperationReader, operationID string) (UploadImportResponse, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
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
		}
	}
}
