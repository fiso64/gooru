package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
	"gooru.local/types"
)

type TagOperation string

const (
	TagOperationAdd    TagOperation = "add"
	TagOperationSet    TagOperation = "set"
	TagOperationRemove TagOperation = "remove"
)

type TagMutationLibrary interface {
	MutateTags(ctx context.Context, operation TagOperation, request TagMutationRequest) (TagMutationResponse, error)
}

type TagMutationRequest struct {
	FileIDs        []string `json:"file_ids,omitempty"`
	Query          string   `json:"query,omitempty"`
	SelectionID    string   `json:"selection_id,omitempty"`
	IncludeFileIDs []string `json:"include_file_ids,omitempty"`
	ExcludeFileIDs []string `json:"exclude_file_ids,omitempty"`
	Tags           []string `json:"tags"`
	Verbose        bool     `json:"verbose,omitempty"`
}

type TagMutationResponse struct {
	Operation     TagOperation        `json:"operation"`
	Selector      TagMutationSelector `json:"selector"`
	MatchedFiles  int                 `json:"matched_files,omitempty"`
	AffectedCount int                 `json:"affected_count"`
	Notifications []NotificationDTO   `json:"notifications,omitempty"`
}

type TagMutationSelector struct {
	FileIDs        []string `json:"file_ids,omitempty"`
	Query          string   `json:"query,omitempty"`
	SelectionID    string   `json:"selection_id,omitempty"`
	IncludeFileIDs []string `json:"include_file_ids,omitempty"`
	ExcludeFileIDs []string `json:"exclude_file_ids,omitempty"`
}

type NotificationDTO struct {
	Kind         string   `json:"kind"`
	OriginalPath string   `json:"original_path,omitempty"`
	OldPath      string   `json:"old_path,omitempty"`
	NewPath      string   `json:"new_path,omitempty"`
	OrphanedTags []string `json:"orphaned_tags,omitempty"`
}

func (s *Server) handleMutateTags(w http.ResponseWriter, r *http.Request) {
	operation, ok := tagOperationForMethod(r.Method)
	if !ok {
		w.Header().Set("Allow", "POST, PUT, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	legacyMutator, legacyOK := s.library.(TagMutationLibrary)
	durableMutator, durableOK := s.library.(durableTagMutationLibrary)
	if s.library == nil || (!legacyOK && !durableOK) {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "tag mutation service is not configured", nil)
		return
	}
	request, err := decodeTagMutationRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if err := validateTagMutationRequest(operation, request); err != nil {
		if errors.Is(err, core.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}

	selector := TagMutationSelector{
		FileIDs:        request.FileIDs,
		Query:          request.Query,
		SelectionID:    request.SelectionID,
		IncludeFileIDs: request.IncludeFileIDs,
		ExcludeFileIDs: request.ExcludeFileIDs,
	}
	target, err := s.resolveBulkFileTarget(r.Context(), fileSelectionOwnerID(r), bulkFileTarget{
		FileIDs:        request.FileIDs,
		Query:          request.Query,
		SelectionID:    request.SelectionID,
		IncludeFileIDs: request.IncludeFileIDs,
		ExcludeFileIDs: request.ExcludeFileIDs,
	})
	if errors.Is(err, errFileSelectionNotFound) {
		writeError(w, http.StatusGone, "selection_expired", "file selection has expired", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve file selection", nil)
		return
	}
	resolvedRequest := request
	resolvedRequest.FileIDs = target.FileIDs
	resolvedRequest.Query = target.Query
	resolvedRequest.SelectionID = target.SelectionID
	resolvedRequest.IncludeFileIDs = target.IncludeFileIDs
	resolvedRequest.ExcludeFileIDs = target.ExcludeFileIDs
	if request.SelectionID == "" {
		if err := s.validateTagMutationFiles(r.Context(), request); err != nil {
			if errors.Is(err, ErrNotFound) {
				writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
				return
			}
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load file", nil)
			return
		}
	}

	if !durableOK {
		if PreferAsync(r) {
			writeError(w, http.StatusServiceUnavailable, "service_unavailable", "durable tag mutation service is not configured", nil)
			return
		}
		if request.SelectionID != "" && len(resolvedRequest.FileIDs) == 0 {
			writeJSON(w, http.StatusOK, TagMutationResponse{Operation: operation, Selector: selector})
			return
		}
		response, err := legacyMutator.MutateTags(r.Context(), operation, resolvedRequest)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to mutate tags", nil)
			return
		}
		response.Selector = selector
		writeJSON(w, http.StatusOK, response)
		return
	}
	if s.backgroundOperations == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "background operation service is not configured", nil)
		return
	}
	operationState, err := durableMutator.createBackgroundTagMutation(
		r.Context(),
		operation,
		selector,
		resolvedRequest,
		defaultDurableTagMutationPendingLimit,
	)
	if err != nil {
		writeDurableTagMutationAdmissionError(w, err)
		return
	}
	state, found, err := s.backgroundOperations.GetBackgroundOperation(operationState.ID)
	if err != nil || !found {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load tag mutation operation", nil)
		return
	}
	if PreferAsync(r) {
		w.Header().Set("Location", "/api/v1/operations/"+operationState.ID)
		writeJSON(w, http.StatusAccepted, backgroundOperationDTO(state))
		return
	}
	response, err := waitForDurableTagMutation(r.Context(), s.backgroundOperations, operationState.ID)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			_, _ = s.cancelBackgroundOperation(operationState.ID)
			writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to mutate tags", nil)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func tagOperationForMethod(method string) (TagOperation, bool) {
	switch method {
	case http.MethodPost:
		return TagOperationAdd, true
	case http.MethodPut:
		return TagOperationSet, true
	case http.MethodDelete:
		return TagOperationRemove, true
	default:
		return "", false
	}
}

func (s *Server) validateTagMutationFiles(ctx context.Context, request TagMutationRequest) error {
	ids := append(append([]string(nil), request.FileIDs...), request.ExcludeFileIDs...)
	for _, encoded := range ids {
		if _, err := s.getFileByPublicID(ctx, encoded); err != nil {
			return err
		}
	}
	return nil
}

func decodeTagMutationRequest(r *http.Request) (TagMutationRequest, error) {
	defer r.Body.Close()
	var request TagMutationRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		if errors.Is(err, io.EOF) {
			return request, errors.New("request body is required")
		}
		return request, fmt.Errorf("invalid JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return request, errors.New("request body must contain a single JSON object")
	}
	request.Query = strings.TrimSpace(request.Query)
	request.SelectionID = strings.TrimSpace(request.SelectionID)
	request.FileIDs = normalizeStrings(request.FileIDs)
	request.IncludeFileIDs = normalizeStrings(request.IncludeFileIDs)
	request.ExcludeFileIDs = normalizeStrings(request.ExcludeFileIDs)
	request.Tags = normalizeStrings(request.Tags)
	return request, nil
}

func validateTagMutationRequest(operation TagOperation, request TagMutationRequest) error {
	hasIDs := len(request.FileIDs) > 0
	hasQuery := strings.TrimSpace(request.Query) != ""
	hasSelection := strings.TrimSpace(request.SelectionID) != ""
	selectorCount := 0
	if hasIDs {
		selectorCount++
	}
	if hasQuery {
		selectorCount++
	}
	if hasSelection {
		selectorCount++
	}
	if selectorCount != 1 {
		return errors.New("provide exactly one selector: file_ids, query, or selection_id")
	}
	if operation == TagOperationAdd && len(request.Tags) == 0 {
		return errors.New("tags are required for add operations")
	}
	if len(request.Tags) > 0 {
		if err := query.ValidateTags(request.Tags); err != nil {
			return err
		}
	}
	if len(request.IncludeFileIDs) > 0 && !hasSelection {
		return errors.New("include_file_ids requires a selection_id selector")
	}
	if len(request.ExcludeFileIDs) > 0 && !hasQuery && !hasSelection {
		return errors.New("exclude_file_ids requires a query or selection_id selector")
	}
	if hasIDs {
		if err := rejectDuplicateFileIDs(request.FileIDs, "file id"); err != nil {
			return err
		}
	}
	if err := rejectDuplicateFileIDs(request.IncludeFileIDs, "included file id"); err != nil {
		return err
	}
	if err := rejectDuplicateFileIDs(request.ExcludeFileIDs, "excluded file id"); err != nil {
		return err
	}
	if err := rejectOverlappingFileIDs(request.IncludeFileIDs, request.ExcludeFileIDs); err != nil {
		return err
	}
	if hasQuery {
		ast, err := query.Parse(request.Query)
		if err != nil {
			return fmt.Errorf("%w: could not parse query: %v", core.ErrInvalidQuery, err)
		}
		if err := query.ValidateAST(ast); err != nil {
			return fmt.Errorf("%w: invalid tag in query: %v", core.ErrInvalidQuery, err)
		}
	}
	return nil
}

func mergeSnapshotSelectionIDs(snapshotIDs, includedIDs, excludedIDs []string) []string {
	// Snapshot IDs are sorted by fileSelectionStore. Avoid constructing a
	// second million-entry membership map for the common no-override path,
	// and use binary search for the small explicit include set.
	if len(includedIDs) == 0 && len(excludedIDs) == 0 {
		return snapshotIDs
	}
	excluded := make(map[string]struct{}, len(excludedIDs))
	for _, id := range excludedIDs {
		excluded[id] = struct{}{}
	}
	selected := make([]string, 0, len(snapshotIDs)+len(includedIDs))
	for _, id := range snapshotIDs {
		if _, skip := excluded[id]; skip {
			continue
		}
		selected = append(selected, id)
	}
	for _, id := range includedIDs {
		if _, skip := excluded[id]; skip {
			continue
		}
		index := sort.SearchStrings(snapshotIDs, id)
		if index < len(snapshotIDs) && snapshotIDs[index] == id {
			continue
		}
		selected = append(selected, id)
	}
	return selected
}

func (l *GooruLibrary) MutateTags(ctx context.Context, operation TagOperation, request TagMutationRequest) (TagMutationResponse, error) {
	if err := ctx.Err(); err != nil {
		return TagMutationResponse{}, err
	}
	response := TagMutationResponse{
		Operation: operation,
		Selector: TagMutationSelector{
			FileIDs:        request.FileIDs,
			Query:          request.Query,
			SelectionID:    request.SelectionID,
			IncludeFileIDs: request.IncludeFileIDs,
			ExcludeFileIDs: request.ExcludeFileIDs,
		},
	}
	if len(request.FileIDs) > 0 {
		paths := make([]string, 0, len(request.FileIDs))
		for _, encoded := range request.FileIDs {
			file, err := l.GetFileByPublicID(ctx, encoded)
			if err != nil {
				return TagMutationResponse{}, err
			}
			paths = append(paths, file.Path)
		}
		result, err := l.mutateTagPaths(operation, paths, request.Tags)
		if err != nil {
			return TagMutationResponse{}, err
		}
		response.MatchedFiles = len(paths)
		response.AffectedCount = result.AffectedCount
		response.Notifications = notificationDTOs(result.Notifications)
		return response, nil
	}

	excludedHashes := make([]string, 0, len(request.ExcludeFileIDs))
	for _, encoded := range request.ExcludeFileIDs {
		file, err := l.GetFileByPublicID(ctx, encoded)
		if err != nil {
			return TagMutationResponse{}, err
		}
		excludedHashes = append(excludedHashes, file.Hash)
	}
	affected, err := l.mutateTagQueryExcluding(operation, request.Query, request.Tags, excludedHashes)
	if err != nil {
		return TagMutationResponse{}, err
	}
	response.AffectedCount = affected
	return response, nil
}

func (l *GooruLibrary) mutateTagPaths(operation TagOperation, paths []string, tags []string) (types.TagOperationResult, error) {
	switch operation {
	case TagOperationAdd:
		return l.client.TagFiles(paths, tags, nil, true)
	case TagOperationSet:
		return l.client.SetTagsForFiles(paths, tags, nil, true)
	case TagOperationRemove:
		return l.client.UntagFiles(paths, tags, nil, true)
	default:
		return types.TagOperationResult{}, fmt.Errorf("unsupported tag operation %q", operation)
	}
}

func (l *GooruLibrary) mutateTagQuery(operation TagOperation, expression string, tags []string) (int, error) {
	return l.mutateTagQueryExcluding(operation, expression, tags, nil)
}

func (l *GooruLibrary) mutateTagQueryExcluding(operation TagOperation, expression string, tags, excludedHashes []string) (int, error) {
	switch operation {
	case TagOperationAdd:
		if len(excludedHashes) == 0 {
			return l.client.TagFilesByQuery(expression, tags)
		}
		return l.client.TagFilesByQueryExcluding(expression, tags, excludedHashes)
	case TagOperationSet:
		if len(excludedHashes) == 0 {
			return l.client.SetTagsForFilesByQuery(expression, tags)
		}
		return l.client.SetTagsForFilesByQueryExcluding(expression, tags, excludedHashes)
	case TagOperationRemove:
		if len(excludedHashes) == 0 {
			return l.client.UntagFilesByQuery(expression, tags)
		}
		return l.client.UntagFilesByQueryExcluding(expression, tags, excludedHashes)
	default:
		return 0, fmt.Errorf("unsupported tag operation %q", operation)
	}
}

func notificationDTOs(notifications []types.Notification) []NotificationDTO {
	if len(notifications) == 0 {
		return nil
	}
	out := make([]NotificationDTO, 0, len(notifications))
	for _, notification := range notifications {
		out = append(out, NotificationDTO{
			Kind:         notificationKindString(notification.Kind),
			OriginalPath: notification.OriginalPath,
			OldPath:      notification.OldPath,
			NewPath:      notification.NewPath,
			OrphanedTags: nonNilStrings(notification.OrphanedTags),
		})
	}
	return out
}

func notificationKindString(kind types.NotificationKind) string {
	switch kind {
	case types.NotificationKindMoveDetected:
		return "move_detected"
	case types.NotificationKindModified:
		return "modified"
	default:
		return "unknown"
	}
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
