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

type backgroundOperationSummaryReader interface {
	GetBackgroundOperationResultSummaries([]string) (map[string]core.BackgroundOperationResultSummary, error)
}

type backgroundOperationHistoryClearer interface {
	ClearTerminalBackgroundOperations() (int64, error)
}

type backgroundOperationCheckpointReader interface {
	GetBackgroundOperationCheckpoint(string, any) (bool, error)
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
	ID                      string                    `json:"id"`
	Kind                    string                    `json:"kind"`
	Status                  core.BackgroundWorkStatus `json:"status"`
	Stage                   string                    `json:"stage,omitempty"`
	ProgressTotal           int64                     `json:"progress_total"`
	ProgressCompleted       int64                     `json:"progress_completed"`
	ProgressCompletedPrefix int64                     `json:"progress_completed_prefix,omitempty"`
	ProgressFailed          int64                     `json:"progress_failed"`
	Progress                *float64                  `json:"progress,omitempty"`
	CreatedAt               time.Time                 `json:"created_at"`
	StartedAt               *time.Time                `json:"started_at,omitempty"`
	FinishedAt              *time.Time                `json:"finished_at,omitempty"`
	ErrorCode               string                    `json:"error_code,omitempty"`
	ErrorMessage            string                    `json:"error_message,omitempty"`
	ResultOutcome           string                    `json:"-"`
	ResultAffectedCount     *int64                    `json:"-"`
	ResultFailedCount       *int64                    `json:"-"`
	Result                  json.RawMessage           `json:"result,omitempty"`
}

type BackgroundOperationListResponse struct {
	Items       []BackgroundOperationDTO `json:"items"`
	ActiveCount *int                     `json:"active_count,omitempty"`
	TotalCount  *int                     `json:"total_count,omitempty"`
}

type BackgroundOperationClearResponse struct {
	Cleared int64 `json:"cleared"`
}

func (l *GooruLibrary) GetBackgroundOperation(operationID string) (core.BackgroundOperationState, bool, error) {
	return l.client.GetBackgroundOperation(operationID)
}

func (l *GooruLibrary) ListBackgroundOperations(options core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error) {
	return l.client.ListBackgroundOperations(options)
}

func (l *GooruLibrary) GetBackgroundOperationResultSummaries(operationIDs []string) (map[string]core.BackgroundOperationResultSummary, error) {
	return l.client.GetBackgroundOperationResultSummaries(operationIDs)
}

func (l *GooruLibrary) ClearTerminalBackgroundOperations() (int64, error) {
	return l.client.ClearTerminalBackgroundOperations()
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
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.backgroundOperations == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation service is not configured", nil)
		return
	}
	if r.Method == http.MethodDelete {
		clearer, ok := s.backgroundOperations.(backgroundOperationHistoryClearer)
		if !ok {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation history clearing is not configured", nil)
			return
		}
		cleared, err := clearer.ClearTerminalBackgroundOperations()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to clear completed background operations", nil)
			return
		}
		writeJSON(w, http.StatusOK, BackgroundOperationClearResponse{Cleared: cleared})
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
			dto := s.backgroundOperationDTO(operation)
			if operation.Status == core.BackgroundWorkCompleted {
				var result json.RawMessage
				found, err := s.backgroundOperationResult(operation, &result)
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
	offset := 0
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer", nil)
			return
		}
		offset = parsed
	}
	operations, err := s.backgroundOperations.ListBackgroundOperations(core.BackgroundOperationListOptions{VisibleOnly: true, Limit: limit, Offset: offset})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load background operations", nil)
		return
	}
	activeCount, err := s.activeBackgroundOperationCount(operations)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to count active background operations", nil)
		return
	}
	totalCount, err := s.backgroundOperationTotalCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to count background operations", nil)
		return
	}
	summaries := make(map[string]core.BackgroundOperationResultSummary)
	if reader, ok := s.backgroundOperations.(backgroundOperationSummaryReader); ok {
		ids := make([]string, 0, len(operations))
		for _, operation := range operations {
			if operation.Visible && operation.Status == core.BackgroundWorkCompleted {
				ids = append(ids, operation.ID)
			}
		}
		summaries, err = reader.GetBackgroundOperationResultSummaries(ids)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load background operation summaries", nil)
			return
		}
	}
	items := make([]BackgroundOperationDTO, 0, len(operations))
	for _, operation := range operations {
		if !operation.Visible {
			continue
		}
		dto := s.backgroundOperationListDTO(operation)
		if summary, ok := summaries[operation.ID]; ok {
			dto.ResultOutcome = summary.Outcome
			dto.ResultAffectedCount = summary.AffectedCount
			dto.ResultFailedCount = summary.FailedCount
		}
		items = append(items, dto)
	}
	writeJSON(w, http.StatusOK, BackgroundOperationListResponse{Items: items, ActiveCount: &activeCount, TotalCount: totalCount})
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
		writeJSON(w, http.StatusAccepted, s.backgroundOperationDTO(operation))
		return
	}
	dto := s.backgroundOperationDTO(operation)
	if operation.Status == core.BackgroundWorkCompleted {
		var result json.RawMessage
		found, err := s.backgroundOperationResult(operation, &result)
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

// backgroundOperationListDTO avoids loading terminal upload checkpoints. An
// imported checkpoint retains the full per-file response for crash recovery,
// which is useful for explicit operation reads but defeats the bounded list API.
func (s *Server) backgroundOperationListDTO(operation core.BackgroundOperationState) BackgroundOperationDTO {
	if operation.Status == core.BackgroundWorkCompleted {
		return backgroundOperationDTO(operation)
	}
	return s.backgroundOperationDTO(operation)
}

func (s *Server) backgroundOperationDTO(operation core.BackgroundOperationState) BackgroundOperationDTO {
	dto := backgroundOperationDTO(operation)
	if operation.Kind != backgroundUploadImportOperationKind || s.backgroundOperations == nil {
		return dto
	}
	if operation.ProgressTotal > 1 {
		if segmented, ok := s.segmentedUploadOperationDTO(operation, dto); ok {
			return segmented
		}
	}
	reader, ok := s.backgroundOperations.(backgroundOperationCheckpointReader)
	if !ok {
		return dto
	}
	var checkpoint backgroundUploadCheckpoint
	found, err := reader.GetBackgroundOperationCheckpoint(operation.ID, &checkpoint)
	if err != nil || !found {
		return dto
	}
	if checkpoint.Phase == backgroundUploadPhaseReceiving {
		dto.Stage = "receiving"
		if dto.Status == core.BackgroundWorkPending {
			dto.Status = core.BackgroundWorkRunning
		}
		if checkpoint.TransportBytesTotal > 0 {
			received := checkpoint.TransportBytesReceived
			if received < 0 {
				received = 0
			}
			if received > checkpoint.TransportBytesTotal {
				received = checkpoint.TransportBytesTotal
			}
			progress := 0.5 * float64(received) / float64(checkpoint.TransportBytesTotal)
			dto.Progress = &progress
		}
		return dto
	}
	dto.Stage = "importing"
	if checkpoint.FileTotal <= 0 {
		return dto
	}
	total := int64(checkpoint.FileTotal)
	completed := int64(checkpoint.FilesCompleted)
	completedPrefix := int64(checkpoint.FilesCompletedPrefix)
	if operation.Status == core.BackgroundWorkCompleted {
		completed = total
		completedPrefix = total
	}
	if completed < 0 {
		completed = 0
	}
	if completed > total {
		completed = total
	}
	if completedPrefix < 0 {
		completedPrefix = 0
	}
	if completedPrefix > completed {
		completedPrefix = completed
	}
	dto.ProgressTotal = total
	dto.ProgressCompleted = completed
	dto.ProgressCompletedPrefix = completedPrefix
	progress := 0.5 + 0.5*float64(completed)/float64(total)
	if operation.Status == core.BackgroundWorkCompleted {
		progress = 1
	}
	if progress > 1 {
		progress = 1
	}
	dto.Progress = &progress
	return dto
}
