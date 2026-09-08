package serve

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	core "gooru.local/gooru"
)

const defaultBackgroundOperationAPILimit = 100

type backgroundOperationReader interface {
	GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)
	ListBackgroundOperations(core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error)
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
	limit := defaultBackgroundOperationAPILimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 1000 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 1000", nil)
			return
		}
		limit = parsed
	}
	operations, err := s.backgroundOperations.ListBackgroundOperations(core.BackgroundOperationListOptions{
		VisibleOnly: true,
		Limit:       limit,
	})
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
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
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
	writeJSON(w, http.StatusOK, backgroundOperationDTO(operation))
}

func backgroundOperationDTO(operation core.BackgroundOperationState) BackgroundOperationDTO {
	return BackgroundOperationDTO{
		ID:                operation.ID,
		Kind:              operation.Kind,
		Status:            operation.Status,
		ProgressTotal:     operation.ProgressTotal,
		ProgressCompleted: operation.ProgressCompleted,
		ProgressFailed:    operation.ProgressFailed,
		CreatedAt:         operation.CreatedAt,
		StartedAt:         operation.StartedAt,
		FinishedAt:        operation.FinishedAt,
		ErrorCode:         operation.ErrorCode,
		ErrorMessage:      operation.ErrorMessage,
	}
}
