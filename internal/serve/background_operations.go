package serve

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	core "gooru.local/gooru"
)

const (
	defaultBackgroundOperationAPILimit = 100
	maxBackgroundOperationStatusBatch  = 64
)

type backgroundOperationReader interface {
	GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)
	ListBackgroundOperations(core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error)
	CancelBackgroundOperation(string) (bool, error)
	GetBackgroundOperationResult(string, any) (bool, error)
}

type backgroundOperationProducer interface {
	CreateBackgroundOperationWithPendingLimit(core.BackgroundOperationRequest, int) (core.BackgroundOperation, error)
	EnqueueBackgroundTask(core.BackgroundTaskRequest) (core.BackgroundTask, bool, error)
	SetBackgroundOperationVisible(string, bool) error
	SetBackgroundOperationCheckpoint(string, any) error
	GetBackgroundOperationCheckpoint(string, any) (bool, error)
	SetBackgroundOperationResult(string, any) error
	CancelBackgroundOperation(string) (bool, error)
}

type BackgroundOperationDTO struct {
	ID                string                    `json:"id"`
	Kind              string                    `json:"kind"`
	Status            core.BackgroundWorkStatus `json:"status"`
	ProgressTotal     int64                     `json:"progress_total"`
	ProgressCompleted int64                     `json:"progress_completed"`
	ProgressFailed    int64                     `json:"progress_failed"`
	CreatedAt         time.Time                 `json:"created_at"`
	StartedAt         *time.Time                `json:"started_at,omitempty"`
	FinishedAt        *time.Time                `json:"finished_at,omitempty"`
	ErrorCode         string                    `json:"error_code,omitempty"`
	ErrorMessage      string                    `json:"error_message,omitempty"`
	Result            json.RawMessage           `json:"result,omitempty"`
}

type BackgroundOperationListResponse struct {
	Items []BackgroundOperationDTO `json:"items"`
}

func (l *GooruLibrary) GetBackgroundOperation(operationID string) (core.BackgroundOperationState, bool, error) {
	return l.client.GetBackgroundOperation(operationID)
}

func (l *GooruLibrary) ListBackgroundOperations(options core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error) {
	return l.client.ListBackgroundOperations(options)
}

func (l *GooruLibrary) CancelBackgroundOperation(operationID string) (bool, error) {
	return l.client.CancelBackgroundOperation(operationID)
}

func (l *GooruLibrary) GetBackgroundOperationResult(operationID string, destination any) (bool, error) {
	operation, found, err := l.client.GetBackgroundOperation(operationID)
	if err != nil {
		return false, err
	}
	if found && operation.Kind == core.BackgroundTagMutationOperationKind {
		if operation.Status != core.BackgroundWorkCompleted {
			return false, nil
		}
		response, found, err := l.backgroundTagMutationResponse(operationID)
		if err != nil || !found {
			return found, err
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			return false, err
		}
		if err := json.Unmarshal(encoded, destination); err != nil {
			return false, err
		}
		return true, nil
	}
	return l.client.GetBackgroundOperationResult(operationID, destination)
}

func (l *GooruLibrary) CreateBackgroundOperationWithPendingLimit(request core.BackgroundOperationRequest, maxPending int) (core.BackgroundOperation, error) {
	return l.client.CreateBackgroundOperationWithPendingLimit(request, maxPending)
}

func (l *GooruLibrary) EnqueueBackgroundTask(request core.BackgroundTaskRequest) (core.BackgroundTask, bool, error) {
	return l.client.EnqueueBackgroundTask(request)
}

func (l *GooruLibrary) SetBackgroundOperationVisible(operationID string, visible bool) error {
	return l.client.SetBackgroundOperationVisible(operationID, visible)
}

func (l *GooruLibrary) SetBackgroundOperationCheckpoint(operationID string, checkpoint any) error {
	return l.client.SetBackgroundOperationCheckpoint(operationID, checkpoint)
}

func (l *GooruLibrary) GetBackgroundOperationCheckpoint(operationID string, destination any) (bool, error) {
	return l.client.GetBackgroundOperationCheckpoint(operationID, destination)
}

func (l *GooruLibrary) SetBackgroundOperationResult(operationID string, result any) error {
	return l.client.SetBackgroundOperationResult(operationID, result)
}

func (s *Server) handleOperations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.backgroundOperations == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation service is not configured", nil)
		return
	}
	if rawIDs, ok := r.URL.Query()["id"]; ok {
		ids := make([]string, 0, len(rawIDs))
		seen := make(map[string]struct{}, len(rawIDs))
		for _, rawID := range rawIDs {
			id := strings.TrimSpace(rawID)
			if id == "" {
				writeError(w, http.StatusBadRequest, "invalid_request", "operation id must not be blank", nil)
				return
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
			if len(ids) > maxBackgroundOperationStatusBatch {
				writeError(w, http.StatusBadRequest, "invalid_request", "at most 64 operation ids may be requested", nil)
				return
			}
		}
		items := make([]BackgroundOperationDTO, 0, len(ids))
		for _, id := range ids {
			operation, found, err := s.backgroundOperations.GetBackgroundOperation(id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to load background operation", nil)
				return
			}
			if !found || !operation.Visible {
				continue
			}
			dto := backgroundOperationDTO(operation)
			if operation.Status == core.BackgroundWorkCompleted {
				var result json.RawMessage
				found, err := s.backgroundOperations.GetBackgroundOperationResult(id, &result)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "internal_error", "failed to load background operation result", nil)
					return
				}
				if found {
					dto.Result = result
				}
			}
			items = append(items, dto)
		}
		writeJSON(w, http.StatusOK, BackgroundOperationListResponse{Items: items})
		return
	}
	limit := defaultBackgroundOperationAPILimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 1000", nil)
			return
		}
		limit = parsed
	}
	operations, err := s.backgroundOperations.ListBackgroundOperations(core.BackgroundOperationListOptions{VisibleOnly: true, Limit: limit})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load background operations", nil)
		return
	}
	items := make([]BackgroundOperationDTO, 0, len(operations))
	for _, operation := range operations {
		if operation.Visible {
			items = append(items, backgroundOperationDTO(operation))
		}
	}
	writeJSON(w, http.StatusOK, BackgroundOperationListResponse{Items: items})
}

func (s *Server) handleOperation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.backgroundOperations == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation service is not configured", nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "operation not found", nil)
		return
	}
	operation, found, err := s.backgroundOperations.GetBackgroundOperation(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load background operation", nil)
		return
	}
	if !found || !operation.Visible {
		writeError(w, http.StatusNotFound, "not_found", "operation not found", nil)
		return
	}
	if r.Method == http.MethodDelete {
		canceled, err := s.cancelBackgroundOperation(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to cancel background operation", nil)
			return
		}
		if !canceled {
			writeError(w, http.StatusConflict, "operation_not_active", "operation is not active", nil)
			return
		}
		operation, found, err = s.backgroundOperations.GetBackgroundOperation(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load canceled background operation", nil)
			return
		}
		if !found || !operation.Visible {
			writeError(w, http.StatusNotFound, "not_found", "operation not found", nil)
			return
		}
		writeJSON(w, http.StatusAccepted, backgroundOperationDTO(operation))
		return
	}
	dto := backgroundOperationDTO(operation)
	if operation.Status == core.BackgroundWorkCompleted {
		var result json.RawMessage
		found, err := s.backgroundOperations.GetBackgroundOperationResult(id, &result)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load background operation result", nil)
			return
		}
		if found {
			dto.Result = result
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

func backgroundOperationDTO(operation core.BackgroundOperationState) BackgroundOperationDTO {
	return BackgroundOperationDTO{ID: operation.ID, Kind: operation.Kind, Status: operation.Status, ProgressTotal: operation.ProgressTotal, ProgressCompleted: operation.ProgressCompleted, ProgressFailed: operation.ProgressFailed, CreatedAt: operation.CreatedAt, StartedAt: operation.StartedAt, FinishedAt: operation.FinishedAt, ErrorCode: operation.ErrorCode, ErrorMessage: operation.ErrorMessage}
}
