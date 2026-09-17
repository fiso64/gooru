package serve

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"gooru.local/types"
)

const (
	fileDownloadLookupBatch    = 256
	fileDownloadCopyBufferSize = 256 * 1024
)

type fileDownloadRequest struct {
	FileIDs        []string `json:"file_ids,omitempty"`
	SelectionID    string   `json:"selection_id,omitempty"`
	IncludeFileIDs []string `json:"include_file_ids,omitempty"`
	ExcludeFileIDs []string `json:"exclude_file_ids,omitempty"`
}

type fileDownloadCreateResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (s *Server) handleFileDownloads(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/api/v1/file-downloads" {
		writeError(w, http.StatusNotFound, "not_found", "file download endpoint not found", nil)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	if _, ok := s.library.(batchPublicFileLookupLibrary); !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "bulk file lookup is not configured", nil)
		return
	}

	request, err := decodeFileDownloadRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	request.FileIDs = normalizeStrings(request.FileIDs)
	request.SelectionID = strings.TrimSpace(request.SelectionID)
	request.IncludeFileIDs = normalizeStrings(request.IncludeFileIDs)
	request.ExcludeFileIDs = normalizeStrings(request.ExcludeFileIDs)
	if err := validateFileDownloadRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}

	ownerID := fileSelectionOwnerID(r)
	if request.SelectionID != "" {
		if _, err := s.fileSelections.members(ownerID, request.SelectionID, nil); errors.Is(err, errFileSelectionNotFound) {
			writeError(w, http.StatusGone, "selection_expired", "file selection has expired", nil)
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to validate file selection", nil)
			return
		}
	}

	id, err := s.fileDownloads.create(ownerID, bulkFileTarget{
		FileIDs:        request.FileIDs,
		SelectionID:    request.SelectionID,
		IncludeFileIDs: request.IncludeFileIDs,
		ExcludeFileIDs: request.ExcludeFileIDs,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare file download", nil)
		return
	}
	writeJSON(w, http.StatusCreated, fileDownloadCreateResponse{
		ID:  id,
		URL: "/api/v1/file-downloads/" + id,
	})
}

func decodeFileDownloadRequest(r *http.Request) (fileDownloadRequest, error) {
	defer r.Body.Close()
	var request fileDownloadRequest
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
	return request, nil
}

func validateFileDownloadRequest(request fileDownloadRequest) error {
	hasIDs := len(request.FileIDs) > 0
	hasSelection := request.SelectionID != ""
	if hasIDs == hasSelection {
		return errors.New("provide exactly one selector: file_ids or selection_id")
	}
	if len(request.IncludeFileIDs) > 0 && !hasSelection {
		return errors.New("include_file_ids requires a selection_id selector")
	}
	if len(request.ExcludeFileIDs) > 0 && !hasSelection {
		return errors.New("exclude_file_ids requires a selection_id selector")
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
	return rejectOverlappingFileIDs(request.IncludeFileIDs, request.ExcludeFileIDs)
}

func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/file-downloads/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "not_found", "file download not found", nil)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}

	ownerID := fileSelectionOwnerID(r)
	target, err := s.fileDownloads.resolve(ownerID, id)
	if errors.Is(err, errFileDownloadNotFound) {
		writeError(w, http.StatusGone, "download_expired", "file download has expired", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve file download", nil)
		return
	}
	target, err = s.resolveBulkFileTarget(r.Context(), ownerID, target)
	if errors.Is(err, errFileSelectionNotFound) {
		writeError(w, http.StatusGone, "selection_expired", "file selection has expired", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to resolve selected files", nil)
		return
	}

	started, err := s.streamFileDownloadArchive(w, r.Context(), target.FileIDs)
	if err == nil {
		return
	}
	if !started {
		if errors.Is(err, ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "one or more selected files are no longer available", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to prepare file download", nil)
		return
	}
	// Once ZIP bytes are on the wire we cannot replace the response with JSON.
	// Leaving the central directory incomplete makes the client fail closed
	// instead of presenting a silently partial archive.
	slog.Error("bulk file download stream failed", "download_id", id, "selected_files", len(target.FileIDs), "error", err)
}

func (s *Server) streamFileDownloadArchive(w http.ResponseWriter, ctx context.Context, fileIDs []string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	lookup, ok := s.library.(batchPublicFileLookupLibrary)
	if !ok {
		return false, errors.New("bulk file lookup is not configured")
	}

	firstEnd := fileDownloadLookupBatch
	if firstEnd > len(fileIDs) {
		firstEnd = len(fileIDs)
	}
	var firstFiles []types.FileInfo
	var err error
	if firstEnd > 0 {
		firstFiles, err = lookup.GetFilesByPublicIDs(ctx, fileIDs[:firstEnd])
		if err != nil {
			return false, err
		}
		if err := verifyDownloadBatch(fileIDs[:firstEnd], firstFiles, s.publicFileID); err != nil {
			return false, err
		}
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="gooru-download.zip"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	archive := zip.NewWriter(w)
	names := make(map[string]int)
	copyBuffer := make([]byte, fileDownloadCopyBufferSize)

	writeBatch := func(files []types.FileInfo) error {
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			source, err := s.media.openMediaSource(fileStoragePath(file))
			if err != nil {
				return fmt.Errorf("open file %s: %w", s.publicFileID(file), err)
			}
			header := &zip.FileHeader{
				Name:   uniqueDownloadArchiveName(filepath.Base(file.Path), names),
				Method: zip.Store,
			}
			if file.ModTime > 0 {
				header.SetModTime(time.Unix(file.ModTime, 0))
			}
			entry, err := archive.CreateHeader(header)
			if err == nil {
				_, err = io.CopyBuffer(entry, source, copyBuffer)
			}
			closeErr := source.Close()
			if err != nil {
				return fmt.Errorf("stream file %s: %w", s.publicFileID(file), err)
			}
			if closeErr != nil {
				return fmt.Errorf("close file %s: %w", s.publicFileID(file), closeErr)
			}
		}
		return nil
	}

	if err := writeBatch(firstFiles); err != nil {
		return true, err
	}
	for start := firstEnd; start < len(fileIDs); start += fileDownloadLookupBatch {
		end := start + fileDownloadLookupBatch
		if end > len(fileIDs) {
			end = len(fileIDs)
		}
		files, err := lookup.GetFilesByPublicIDs(ctx, fileIDs[start:end])
		if err != nil {
			return true, err
		}
		if err := verifyDownloadBatch(fileIDs[start:end], files, s.publicFileID); err != nil {
			return true, err
		}
		if err := writeBatch(files); err != nil {
			return true, err
		}
	}
	if err := archive.Close(); err != nil {
		return true, err
	}
	return true, nil
}

func verifyDownloadBatch(fileIDs []string, files []types.FileInfo, publicID func(types.FileInfo) string) error {
	if len(fileIDs) != len(files) {
		return ErrNotFound
	}
	for i, file := range files {
		if publicID(file) != fileIDs[i] {
			return ErrNotFound
		}
	}
	return nil
}

func uniqueDownloadArchiveName(name string, used map[string]int) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "file"
	}
	key := strings.ToLower(name)
	nextSuffix, exists := used[key]
	if !exists {
		used[key] = 2
		return name
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if nextSuffix < 2 {
		nextSuffix = 2
	}
	for suffix := nextSuffix; ; suffix++ {
		candidate := fmt.Sprintf("%s (%d)%s", stem, suffix, ext)
		candidateKey := strings.ToLower(candidate)
		if _, exists := used[candidateKey]; exists {
			continue
		}
		used[key] = suffix + 1
		used[candidateKey] = 2
		return candidate
	}
}
