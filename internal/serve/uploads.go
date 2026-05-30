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
)

type UploadLibrary interface {
	ImportUploadedFiles(ctx context.Context, paths []string, tags []string) (UploadImportResponse, error)
}

type UploadImportResponse struct {
	Files         []UploadedFileDTO `json:"files"`
	AffectedCount int               `json:"affected_count"`
	Notifications []NotificationDTO `json:"notifications,omitempty"`
}

type UploadedFileDTO struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
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
	if !hasUploadDirectory(s.cfg.Uploads.Directories) {
		writeError(w, http.StatusForbidden, "uploads_disabled", "upload directory is not configured", nil)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "multipart upload body is required", nil)
		return
	}
	dir, err := s.uploadDirectory(firstFormValue(r.MultipartForm.Value["directory"]))
	if err != nil {
		writeError(w, http.StatusForbidden, "uploads_disabled", err.Error(), nil)
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

	saved, err := s.saveUploadedFiles(dir, files)
	if err != nil {
		if errors.Is(err, errUploadTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	paths := make([]string, 0, len(saved))
	for _, file := range saved {
		paths = append(paths, file.path)
	}

	cleanup := func() {
		removeSavedUploads(saved)
	}
	job, err := s.jobs.SubmitWithCleanup(r.Context(), "upload_import", PreferAsync(r), func(ctx context.Context) (interface{}, error) {
		response, err := importer.ImportUploadedFiles(ctx, paths, tags)
		if err != nil {
			cleanup()
			return nil, err
		}
		response.Files = uploadedFileDTOs(saved)
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

func (s *Server) uploadDirectory(name string) (string, error) {
	if !s.cfg.Uploads.Enabled {
		return "", errors.New("uploads are disabled")
	}
	name = strings.TrimSpace(name)
	var fallback string
	for _, dir := range s.cfg.Uploads.Directories {
		if strings.TrimSpace(dir.Path) == "" {
			continue
		}
		if fallback == "" {
			fallback = dir.Path
		}
		if name != "" && dir.Name == name {
			return dir.Path, nil
		}
	}
	if name == "" && fallback != "" {
		return fallback, nil
	}
	return "", errors.New("upload directory is not configured")
}

type savedUpload struct {
	name string
	path string
	size int64
}

var errUploadTooLarge = errors.New("uploaded file exceeds max_file_size_bytes")

func (s *Server) saveUploadedFiles(dir string, files []*multipart.FileHeader) ([]savedUpload, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to prepare upload directory")
	}
	saved := make([]savedUpload, 0, len(files))
	for _, header := range files {
		name, err := safeUploadName(header.Filename)
		if err != nil {
			return nil, err
		}
		src, err := header.Open()
		if err != nil {
			removeSavedUploads(saved)
			return nil, fmt.Errorf("failed to read uploaded file")
		}
		dst, path, tmpPath, err := createUploadDestination(dir, name)
		if err != nil {
			_ = src.Close()
			removeSavedUploads(saved)
			return nil, err
		}
		size, copyErr := copyUpload(dst, src, s.cfg.Uploads.MaxFileSizeBytes)
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			removeSavedUploads(saved)
			if copyErr != nil {
				return nil, copyErr
			}
			return nil, fmt.Errorf("failed to write uploaded file")
		}
		if err := commitUploadDestination(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			removeSavedUploads(saved)
			return nil, err
		}
		saved = append(saved, savedUpload{name: filepath.Base(path), path: path, size: size})
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

func uploadedFileDTOs(files []savedUpload) []UploadedFileDTO {
	out := make([]UploadedFileDTO, 0, len(files))
	for _, file := range files {
		out = append(out, UploadedFileDTO{Name: file.name, Size: file.size})
	}
	return out
}

func (l *GooruLibrary) ImportUploadedFiles(ctx context.Context, paths []string, tags []string) (UploadImportResponse, error) {
	if err := ctx.Err(); err != nil {
		return UploadImportResponse{}, err
	}
	result, err := l.client.TagFiles(paths, tags, nil, false)
	if err != nil {
		return UploadImportResponse{}, err
	}
	return UploadImportResponse{
		AffectedCount: result.AffectedCount,
		Notifications: notificationDTOs(result.Notifications),
	}, nil
}
