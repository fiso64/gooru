package serve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

type UploadLibrary interface {
	ImportUploadedFiles(ctx context.Context, files []StagedUpload, tags []string) (UploadImportResponse, error)
}

type UploadImportResponse struct {
	Files         []UploadedFileDTO `json:"files"`
	AffectedCount int               `json:"affected_count"`
	Notifications []NotificationDTO `json:"notifications,omitempty"`
}

type UploadedFileDTO struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	TargetID string `json:"target_id"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type StagedUpload struct {
	Name     string
	Path     string
	Size     int64
	TargetID string
}

type UploadTargetsResponse struct {
	Items []UploadTargetDTO `json:"items"`
}

type UploadTargetDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) handleUploadTargets(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Uploads.Enabled {
		writeJSON(w, http.StatusOK, UploadTargetsResponse{Items: []UploadTargetDTO{}})
		return
	}
	items := make([]UploadTargetDTO, 0, len(s.cfg.Uploads.Targets))
	for _, target := range s.cfg.Uploads.Targets {
		if strings.TrimSpace(target.ID) == "" || strings.TrimSpace(target.Path) == "" {
			continue
		}
		items = append(items, UploadTargetDTO{ID: target.ID, Name: target.Name})
	}
	writeJSON(w, http.StatusOK, UploadTargetsResponse{Items: items})
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	importer, ok := s.library.(UploadLibrary)
	if s.library == nil || !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "upload import service is not configured", nil)
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
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "multipart upload body is required", nil)
		return
	}
	target, err := s.uploadTarget(firstFormValue(r.MultipartForm.Value["target_id"]))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload_target", err.Error(), nil)
		return
	}
	tags := parseUploadTags(r.MultipartForm.Value["tags"])
	if err := query.ValidateTags(tags); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	files := uploadFileHeaders(r.MultipartForm.File)
	if len(files) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "at least one file is required", nil)
		return
	}

	saved, err := s.saveUploadedFiles(target, files)
	if err != nil {
		if errors.Is(err, errUploadTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), uploadErrorDetails(err))
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), uploadErrorDetails(err))
		return
	}
	cleanup := func() {
		removeSavedUploads(saved)
	}
	job, err := s.jobs.SubmitWithCleanup(r.Context(), "upload_import", PreferAsync(r), func(ctx context.Context) (interface{}, error) {
		response, err := importer.ImportUploadedFiles(ctx, stagedUploads(saved), tags)
		if err != nil {
			cleanup()
			return nil, err
		}
		return response, nil
	}, cleanup)
	if PreferAsync(r) && err == nil {
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	if err != nil {
		writeJobSubmitError(w, err, "failed to import uploaded files")
		return
	}
	response, ok := job.Result.(UploadImportResponse)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) uploadTarget(id string) (UploadTarget, error) {
	if !s.cfg.Uploads.Enabled {
		return UploadTarget{}, errors.New("uploads are disabled")
	}
	id = strings.TrimSpace(id)
	var fallback *UploadTarget
	for i := range s.cfg.Uploads.Targets {
		target := s.cfg.Uploads.Targets[i]
		if strings.TrimSpace(target.ID) == "" || strings.TrimSpace(target.Path) == "" {
			continue
		}
		if fallback == nil {
			fallback = &target
		}
		if id != "" && target.ID == id {
			return target, nil
		}
	}
	if id == "" && fallback != nil {
		return *fallback, nil
	}
	return UploadTarget{}, errors.New("upload target is not configured")
}

type savedUpload struct {
	name     string
	path     string
	size     int64
	targetID string
}

var errUploadTooLarge = errors.New("uploaded file exceeds max_file_size_bytes")

type uploadFileError struct {
	name string
	err  error
}

func (e uploadFileError) Error() string {
	if e.name == "" {
		return e.err.Error()
	}
	return fmt.Sprintf("%s: %v", e.name, e.err)
}

func (e uploadFileError) Unwrap() error {
	return e.err
}

func uploadErrorDetails(err error) map[string]string {
	var fileErr uploadFileError
	if errors.As(err, &fileErr) && fileErr.name != "" {
		return map[string]string{"file": fileErr.name}
	}
	return nil
}

func (s *Server) saveUploadedFiles(target UploadTarget, files []*multipart.FileHeader) ([]savedUpload, error) {
	if err := os.MkdirAll(target.Path, 0700); err != nil {
		return nil, fmt.Errorf("failed to prepare upload directory")
	}
	saved := make([]savedUpload, 0, len(files))
	for _, header := range files {
		name, err := safeUploadName(header.Filename)
		if err != nil {
			return nil, uploadFileError{name: header.Filename, err: err}
		}
		src, err := header.Open()
		if err != nil {
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: fmt.Errorf("failed to read uploaded file")}
		}
		dst, path, tmpPath, err := createUploadDestination(target.Path, name)
		if err != nil {
			_ = src.Close()
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		size, copyErr := copyUpload(dst, src, s.cfg.Uploads.MaxFileSizeBytes)
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			removeSavedUploads(saved)
			if copyErr != nil {
				return nil, uploadFileError{name: name, err: copyErr}
			}
			return nil, uploadFileError{name: name, err: fmt.Errorf("failed to write uploaded file")}
		}
		if err := commitUploadDestination(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		saved = append(saved, savedUpload{name: filepath.Base(path), path: path, size: size, targetID: target.ID})
	}
	return saved, nil
}

func removeSavedUploads(files []savedUpload) {
	for _, file := range files {
		_ = os.Remove(file.path)
	}
}

func safeUploadName(name string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	name = filepath.Base(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "", errors.New("uploaded filename is invalid")
	}
	if strings.Contains(name, "/") || strings.Contains(name, string(filepath.Separator)) {
		return "", errors.New("uploaded filename is invalid")
	}
	return name, nil
}

func createUploadDestination(dir string, name string) (*os.File, string, string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = "upload"
	}
	for i := 0; i < 10_000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		path := filepath.Join(dir, candidate)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, "", "", fmt.Errorf("failed to create uploaded file")
		}
		file, err := os.CreateTemp(dir, "."+candidate+".tmp-*")
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to create uploaded file")
		}
		return file, path, file.Name(), nil
	}
	return nil, "", "", errors.New("could not choose a non-conflicting upload filename")
}

func commitUploadDestination(tmpPath string, finalPath string) error {
	if err := os.Link(tmpPath, finalPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("uploaded filename conflicts with an existing file")
		}
		return fmt.Errorf("failed to store uploaded file")
	}
	if err := os.Remove(tmpPath); err != nil {
		_ = os.Remove(finalPath)
		return fmt.Errorf("failed to store uploaded file")
	}
	return nil
}

func copyUpload(dst io.Writer, src io.Reader, maxSize int64) (int64, error) {
	if maxSize <= 0 {
		return io.Copy(dst, src)
	}
	limited := &io.LimitedReader{R: src, N: maxSize + 1}
	n, err := io.Copy(dst, limited)
	if err != nil {
		return n, err
	}
	if n > maxSize {
		return n, errUploadTooLarge
	}
	return n, nil
}

func uploadFileHeaders(files map[string][]*multipart.FileHeader) []*multipart.FileHeader {
	var out []*multipart.FileHeader
	out = append(out, files["files"]...)
	out = append(out, files["file"]...)
	return out
}

func parseUploadTags(values []string) []string {
	var tags []string
	for _, value := range values {
		tags = append(tags, strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\n' || r == '\t'
		})...)
	}
	return normalizeStrings(tags)
}

func firstFormValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func stagedUploads(files []savedUpload) []StagedUpload {
	out := make([]StagedUpload, 0, len(files))
	for _, file := range files {
		out = append(out, StagedUpload{Name: file.name, Path: file.path, Size: file.size, TargetID: file.targetID})
	}
	return out
}

func (l *GooruLibrary) ImportUploadedFiles(ctx context.Context, files []StagedUpload, tags []string) (UploadImportResponse, error) {
	if err := ctx.Err(); err != nil {
		return UploadImportResponse{}, err
	}
	response := UploadImportResponse{Files: make([]UploadedFileDTO, 0, len(files))}
	hashes := make(map[string]string, len(files))
	importPaths := make([]string, 0, len(files))
	responseIndexByPath := make(map[string]int, len(files))
	for _, file := range files {
		dto := UploadedFileDTO{Name: file.Name, Size: file.Size, TargetID: file.TargetID}
		info, status, err := l.client.GetFileInfoForFile(file.Path, false)
		if err != nil {
			dto.Status = "error"
			dto.Error = err.Error()
			response.Files = append(response.Files, dto)
			continue
		}
		if _, ok := hashes[info.Hash]; ok {
			dto.Status = "duplicate_in_batch"
			_ = os.Remove(file.Path)
			response.Files = append(response.Files, dto)
			continue
		}
		hashes[info.Hash] = file.Path
		exists, err := l.client.ContentExists(info.Hash)
		if err != nil {
			return UploadImportResponse{}, err
		}
		if exists || status == types.StatusUntrackedContent || status == types.StatusOK {
			dto.Status = "duplicate_existing"
			_ = os.Remove(file.Path)
			response.Files = append(response.Files, dto)
			continue
		}
		dto.Status = "imported"
		response.Files = append(response.Files, dto)
		responseIndexByPath[file.Path] = len(response.Files) - 1
		importPaths = append(importPaths, file.Path)
	}
	if len(importPaths) == 0 {
		return response, nil
	}
	failures := make(map[string]string)
	result, err := l.client.TagFiles(importPaths, tags, func(filePath string, err error) {
		if err != nil {
			failures[filePath] = err.Error()
		}
	}, false)
	if err != nil {
		return UploadImportResponse{}, err
	}
	for path, message := range failures {
		if i, ok := responseIndexByPath[path]; ok {
			response.Files[i].Status = "error"
			response.Files[i].Error = message
			_ = os.Remove(path)
		}
	}
	response.AffectedCount = result.AffectedCount
	response.Notifications = notificationDTOs(result.Notifications)
	return response, nil
}
