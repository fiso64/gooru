package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	mutator, ok := s.library.(TagMutationLibrary)
	if s.library == nil || !ok {
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
		ExcludeFileIDs: request.ExcludeFileIDs,
	}
	resolvedRequest := request
	if request.SelectionID != "" {
		fileIDs, err := s.fileSelections.resolve(fileSelectionOwnerID(r), request.SelectionID)
		if errors.Is(err, errFileSelectionNotFound) {
			writeError(w, http.StatusGone, "selection_expired", "file selection has expired", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve file selection", nil)
			return
		}
		excluded := make(map[string]struct{}, len(request.ExcludeFileIDs))
		for _, id := range request.ExcludeFileIDs {
			excluded[id] = struct{}{}
		}
		resolvedRequest.FileIDs = make([]string, 0, len(fileIDs))
		for _, id := range fileIDs {
			if _, skip := excluded[id]; !skip {
				resolvedRequest.FileIDs = append(resolvedRequest.FileIDs, id)
			}
		}
		resolvedRequest.Query = ""
		resolvedRequest.SelectionID = ""
		resolvedRequest.ExcludeFileIDs = nil
	} else if err := s.validateTagMutationFiles(r.Context(), request); err != nil {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load file", nil)
		return
	}

	job, err := s.jobs.Submit(r.Context(), "tag_mutation", PreferAsync(r), func(ctx context.Context) (interface{}, error) {
		if request.SelectionID != "" && len(resolvedRequest.FileIDs) == 0 {
			return TagMutationResponse{Operation: operation, Selector: selector}, nil
		}
		response, err := mutator.MutateTags(ctx, operation, resolvedRequest)
		if err != nil {
			return TagMutationResponse{}, err
		}
		response.Selector = selector
		return response, nil
	})
	if PreferAsync(r) && err == nil {
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	if err != nil {
		writeJobSubmitError(w, err, "failed to mutate tags")
		return
	}
	response, ok := job.Result.(TagMutationResponse)
	if !ok {
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
	request.ExcludeFileIDs = normalizeStrings(request.ExcludeFileIDs)
	request.Tags = normalizeStrings(request.Tags)
	return request, nil
}

func validateTagMutationRequest(operation TagOperation, request TagMutationRequest) error {
	hasIDs := len(request.FileIDs) > 0
	hasQuery := strings.TrimSpace(request.Query) != ""
	hasSelection := strings.TrimSpace(request.SelectionID) != ""
	selectorCount := 0
	if hasIDs { selectorCount++ }
	if hasQuery { selectorCount++ }
	if hasSelection { selectorCount++ }
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
	if len(request.ExcludeFileIDs) > 0 && !hasQuery && !hasSelection {
		return errors.New("exclude_file_ids requires a query or selection_id selector")
	}
	if hasIDs {
		seen := map[string]struct{}{}
		for _, id := range request.FileIDs {
			if _, ok := seen[id]; ok {
				return fmt.Errorf("duplicate file id %q", id)
			}
			seen[id] = struct{}{}
		}
	}
	if len(request.ExcludeFileIDs) > 0 {
		seen := map[string]struct{}{}
		for _, id := range request.ExcludeFileIDs {
			if _, ok := seen[id]; ok {
				return fmt.Errorf("duplicate excluded file id %q", id)
			}
			seen[id] = struct{}{}
		}
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
		return l.client.TagFiles(paths, tags, nil, false)
	case TagOperationSet:
		return l.client.SetTagsForFiles(paths, tags, nil, false)
	case TagOperationRemove:
		return l.client.UntagFiles(paths, tags, nil, false)
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
