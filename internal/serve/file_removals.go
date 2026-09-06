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

type FileRemovalRequest struct {
	Mode           string   `json:"mode"`
	FileIDs        []string `json:"file_ids,omitempty"`
	Query          string   `json:"query,omitempty"`
	SelectionID    string   `json:"selection_id,omitempty"`
	IncludeFileIDs []string `json:"include_file_ids,omitempty"`
	ExcludeFileIDs []string `json:"exclude_file_ids,omitempty"`
}

type FileRemovalSelector struct {
	FileIDs        []string `json:"file_ids,omitempty"`
	Query          string   `json:"query,omitempty"`
	SelectionID    string   `json:"selection_id,omitempty"`
	IncludeFileIDs []string `json:"include_file_ids,omitempty"`
	ExcludeFileIDs []string `json:"exclude_file_ids,omitempty"`
}

type FileRemovalResponse struct {
	Mode             string              `json:"mode"`
	Selector         FileRemovalSelector `json:"selector"`
	RemovedLocations int                 `json:"removed_locations"`
}

func (s *Server) handleRemoveFiles(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	if _, ok := s.library.(PublicFileLibrary); !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file mutation service is not configured", nil)
		return
	}
	request, err := decodeFileRemovalRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if err := validateFileRemovalRequest(request); err != nil {
		if errors.Is(err, core.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	files, err := s.resolveFileRemovalSelection(r.Context(), fileSelectionOwnerID(r), request)
	if errors.Is(err, errFileSelectionNotFound) {
		writeError(w, http.StatusGone, "selection_expired", "file selection has expired", nil)
		return
	}
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve selected files", nil)
		return
	}

	// Physical deletion is intentionally all-or-nothing at the policy boundary:
	// validate every selected path before moving or untracking any one of them.
	if request.Mode == "delete" {
		for _, file := range files {
			if !s.canDeleteFilePath(file.Path) {
				writeError(w, http.StatusConflict, "file_not_managed", "one or more selected files are outside configured upload targets; untrack them instead", nil)
				return
			}
		}
	}

	removed := 0
	for _, file := range files {
		publicID := s.publicFileID(file)
		var changed bool
		if request.Mode == "delete" {
			changed, err = s.deleteManagedFile(r.Context(), publicID)
		} else {
			changed, err = s.deleteFileByPublicID(r.Context(), publicID)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to "+request.Mode+" selected files", nil)
			return
		}
		if changed {
			removed++
		}
	}
	writeJSON(w, http.StatusOK, FileRemovalResponse{
		Mode: request.Mode,
		Selector: FileRemovalSelector{
			FileIDs:        request.FileIDs,
			Query:          request.Query,
			SelectionID:    request.SelectionID,
			IncludeFileIDs: request.IncludeFileIDs,
			ExcludeFileIDs: request.ExcludeFileIDs,
		},
		RemovedLocations: removed,
	})
}

func decodeFileRemovalRequest(r *http.Request) (FileRemovalRequest, error) {
	defer r.Body.Close()
	var request FileRemovalRequest
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
	request.Mode = strings.TrimSpace(request.Mode)
	request.Query = strings.TrimSpace(request.Query)
	request.SelectionID = strings.TrimSpace(request.SelectionID)
	request.FileIDs = normalizeStrings(request.FileIDs)
	request.IncludeFileIDs = normalizeStrings(request.IncludeFileIDs)
	request.ExcludeFileIDs = normalizeStrings(request.ExcludeFileIDs)
	return request, nil
}

func validateFileRemovalRequest(request FileRemovalRequest) error {
	if request.Mode != "untrack" && request.Mode != "delete" {
		return errors.New("mode must be untrack or delete")
	}
	hasIDs := len(request.FileIDs) > 0
	hasQuery := request.Query != ""
	hasSelection := request.SelectionID != ""
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
	if len(request.IncludeFileIDs) > 0 && !hasSelection {
		return errors.New("include_file_ids requires a selection_id selector")
	}
	if len(request.ExcludeFileIDs) > 0 && !hasQuery && !hasSelection {
		return errors.New("exclude_file_ids requires a query or selection_id selector")
	}
	if err := rejectDuplicateFileIDs(request.FileIDs, "file id"); err != nil {
		return err
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

func rejectDuplicateFileIDs(ids []string, label string) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate %s %q", label, id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func rejectOverlappingFileIDs(included, excluded []string) error {
	if len(included) == 0 || len(excluded) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(included))
	for _, id := range included {
		seen[id] = struct{}{}
	}
	for _, id := range excluded {
		if _, ok := seen[id]; ok {
			return fmt.Errorf("file id %q cannot be both included and excluded", id)
		}
	}
	return nil
}

func (s *Server) resolveFileRemovalSelection(ctx context.Context, ownerID string, request FileRemovalRequest) ([]types.FileInfo, error) {
	target, err := s.resolveBulkFileTarget(ctx, ownerID, bulkFileTarget{
		FileIDs:        request.FileIDs,
		Query:          request.Query,
		SelectionID:    request.SelectionID,
		IncludeFileIDs: request.IncludeFileIDs,
		ExcludeFileIDs: request.ExcludeFileIDs,
	})
	if err != nil {
		return nil, err
	}
	if len(target.FileIDs) > 0 || request.SelectionID != "" {
		return s.resolveFileIDs(ctx, target.FileIDs, nil)
	}

	excluded := make(map[string]struct{}, len(target.ExcludeFileIDs))
	for _, id := range target.ExcludeFileIDs {
		if _, err := s.getFileByPublicID(ctx, id); err != nil {
			return nil, err
		}
		excluded[id] = struct{}{}
	}
	files, err := s.library.ListFiles(ctx, target.Query)
	if err != nil {
		return nil, err
	}
	selected := make([]types.FileInfo, 0, len(files))
	for _, file := range files {
		if _, skip := excluded[s.publicFileID(file)]; skip {
			continue
		}
		selected = append(selected, file)
	}
	return selected, nil
}

func (s *Server) resolveFileIDs(ctx context.Context, fileIDs, excludedFileIDs []string) ([]types.FileInfo, error) {
	excluded := make(map[string]struct{}, len(excludedFileIDs))
	for _, id := range excludedFileIDs {
		excluded[id] = struct{}{}
	}
	files := make([]types.FileInfo, 0, len(fileIDs))
	for _, id := range fileIDs {
		if _, skip := excluded[id]; skip {
			continue
		}
		file, err := s.getFileByPublicID(ctx, id)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
