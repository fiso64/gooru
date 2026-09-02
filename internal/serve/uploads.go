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
	Name         string
	Path         string
	AnalysisPath string
	Size         int64
	TargetID     string
	Status       string
	Error        string
}

const maxUploadFiles = 100

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
	reservation, err := s.jobs.Reserve(r.Context(), "upload_import")
	if err != nil {
		writeJobSubmitError(w, err, "failed to accept upload")
		return
	}
	submitted := false
	defer func() {
		if !submitted {
			reservation.Release()
		}
	}()
	if limit := s.uploadRequestBodyLimit(); limit > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, limit)
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "upload request body is too large", nil)
			return
		}
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
	if len(files) > maxUploadFiles {
		writeError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("at most %d files are allowed per upload", maxUploadFiles), nil)
		return
	}

	conflictPolicy, err := uploadConflictPolicy(firstFormValue(r.MultipartForm.Value["conflict_policy"]), s.cfg.Uploads.ConflictPolicy)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	saved, err := s.saveUploadedFiles(target, files, conflictPolicy)
	if err != nil {
		if errors.Is(err, errUploadTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), uploadErrorDetails(err))
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), uploadErrorDetails(err))
		return
	}
	if len(saved) == 1 && saved[0].status == "error" && saved[0].error == errUploadTooLarge.Error() {
		fileErr := uploadFileError{name: saved[0].name, err: errUploadTooLarge}
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
		return
	}
	cleanup := func() {
		removeSavedUploads(saved)
	}
	job, err := reservation.Submit(r.Context(), PreferAsync(r), func(ctx context.Context) (interface{}, error) {
		activated, err := activateSavedReplacements(saved)
		if err != nil {
			cleanup()
			return nil, err
		}
		response, err := importer.ImportUploadedFiles(ctx, stagedUploads(saved), tags)
		if err != nil {
			rollbackErr := rollbackSavedReplacements(activated)
			cleanup()
			if rollbackErr != nil {
				return nil, fmt.Errorf("%w; replacement rollback failed: %v", err, rollbackErr)
			}
			return nil, err
		}
		if err := settleSavedReplacements(activated, response); err != nil {
			return nil, err
		}
		return response, nil
	}, cleanup)
	if err == nil {
		submitted = true
	}
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

func (s *Server) uploadRequestBodyLimit() int64 {
	maxFileSize := s.cfg.Uploads.MaxFileSizeBytes
	if maxFileSize <= 0 {
		return 0
	}
	const multipartOverhead int64 = 1 << 20
	const safeLimit int64 = 1 << 62
	if maxFileSize > (safeLimit-multipartOverhead)/maxUploadFiles {
		return safeLimit
	}
	return maxFileSize*maxUploadFiles + multipartOverhead
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
	name            string
	path            string
	destinationPath string
	size            int64
	targetID        string
	status          string
	error           string
	replace         bool
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

func uploadConflictPolicy(requested string, fallback string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		switch requested {
		case "skip", "rename", "replace":
			return requested, nil
		default:
			return "", errors.New("conflict_policy must be one of: skip, rename, replace")
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "rename", nil
	}
	switch fallback {
	case "skip", "rename", "replace", "error":
		return fallback, nil
	default:
		return "", errors.New("configured uploads.conflict_policy is invalid")
	}
}

func (s *Server) saveUploadedFiles(target UploadTarget, files []*multipart.FileHeader, conflictPolicy string) ([]savedUpload, error) {
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
		dst, path, tmpPath, skipped, err := createUploadDestination(target.Path, name, conflictPolicy)
		if err != nil {
			_ = src.Close()
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		if skipped {
			_ = src.Close()
			saved = append(saved, savedUpload{name: name, path: path, destinationPath: path, size: header.Size, targetID: target.ID, status: "skipped"})
			continue
		}
		size, copyErr := copyUpload(dst, src, s.cfg.Uploads.MaxFileSizeBytes)
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if errors.Is(copyErr, errUploadTooLarge) {
				saved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: copyErr.Error()})
				continue
			}
			removeSavedUploads(saved)
			if copyErr != nil {
				return nil, uploadFileError{name: name, err: copyErr}
			}
			return nil, uploadFileError{name: name, err: fmt.Errorf("failed to write uploaded file")}
		}
		if conflictPolicy == "replace" {
			saved = append(saved, savedUpload{
				name:            filepath.Base(path),
				path:            tmpPath,
				destinationPath: path,
				size:            size,
				targetID:        target.ID,
				replace:         true,
			})
			continue
		}
		if err := commitUploadDestination(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		saved = append(saved, savedUpload{name: filepath.Base(path), path: path, destinationPath: path, size: size, targetID: target.ID})
	}
	return saved, nil
}

func removeSavedUploads(files []savedUpload) {
	for _, file := range files {
		if file.status == "skipped" || file.status == "error" {
			continue
		}
		_ = os.Remove(file.path)
	}
}

type activatedReplacement struct {
	finalPath   string
	backupPath  string
	hadOriginal bool
}

type activatedSavedReplacement struct {
	index       int
	replacement activatedReplacement
}

func activateSavedReplacements(files []savedUpload) ([]activatedSavedReplacement, error) {
	activated := make([]activatedSavedReplacement, 0)
	for i, file := range files {
		if !file.replace {
			continue
		}
		replacement, err := activateReplacement(file.path, file.destinationPath)
		if err != nil {
			rollbackErr := rollbackSavedReplacements(activated)
			if rollbackErr != nil {
				return nil, fmt.Errorf("%w; replacement rollback failed: %v", err, rollbackErr)
			}
			return nil, err
		}
		activated = append(activated, activatedSavedReplacement{index: i, replacement: replacement})
	}
	return activated, nil
}

func settleSavedReplacements(activated []activatedSavedReplacement, response UploadImportResponse) error {
	var errs []error
	for _, item := range activated {
		if item.index < len(response.Files) && response.Files[item.index].Status == "imported" {
			commitReplacement(item.replacement)
			continue
		}
		if err := rollbackReplacement(item.replacement); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func rollbackSavedReplacements(activated []activatedSavedReplacement) error {
	var errs []error
	for i := len(activated) - 1; i >= 0; i-- {
		if err := rollbackReplacement(activated[i].replacement); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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

func createUploadDestination(dir string, name string, conflictPolicy string) (*os.File, string, string, bool, error) {
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
			switch conflictPolicy {
			case "skip":
				return nil, path, "", true, nil
			case "error":
				return nil, "", "", false, errors.New("uploaded filename conflicts with an existing file")
			case "replace":
				file, err := os.CreateTemp(dir, "."+candidate+".tmp-*")
				if err != nil {
					return nil, "", "", false, fmt.Errorf("failed to create uploaded file")
				}
				return file, path, file.Name(), false, nil
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, "", "", false, fmt.Errorf("failed to create uploaded file")
		}
		file, err := os.CreateTemp(dir, "."+candidate+".tmp-*")
		if err != nil {
			return nil, "", "", false, fmt.Errorf("failed to create uploaded file")
		}
		return file, path, file.Name(), false, nil
	}
	return nil, "", "", false, errors.New("could not choose a non-conflicting upload filename")
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

func activateReplacement(stagedPath string, finalPath string) (activatedReplacement, error) {
	if stagedPath == "" || finalPath == "" || stagedPath == finalPath {
		return activatedReplacement{}, errors.New("invalid staged replacement")
	}
	replacement := activatedReplacement{finalPath: finalPath, backupPath: stagedPath + ".backup"}
	_ = os.Remove(replacement.backupPath)
	if _, err := os.Stat(finalPath); err == nil {
		if err := os.Rename(finalPath, replacement.backupPath); err != nil {
			return activatedReplacement{}, fmt.Errorf("failed to preserve existing file before replacement")
		}
		replacement.hadOriginal = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return activatedReplacement{}, fmt.Errorf("failed to inspect existing file before replacement")
	}
	if err := os.Rename(stagedPath, finalPath); err != nil {
		if replacement.hadOriginal {
			_ = os.Rename(replacement.backupPath, finalPath)
		}
		return activatedReplacement{}, fmt.Errorf("failed to store uploaded replacement")
	}
	return replacement, nil
}

func rollbackReplacement(replacement activatedReplacement) error {
	if err := os.Remove(replacement.finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove rejected replacement")
	}
	if replacement.hadOriginal {
		if err := os.Rename(replacement.backupPath, replacement.finalPath); err != nil {
			return fmt.Errorf("failed to restore original file after replacement failure")
		}
	} else {
		_ = os.Remove(replacement.backupPath)
	}
	return nil
}

func commitReplacement(replacement activatedReplacement) {
	if replacement.hadOriginal {
		_ = os.Remove(replacement.backupPath)
	}
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
		path := file.path
		if file.replace {
			path = file.destinationPath
		}
		out = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, TargetID: file.targetID, Status: file.status, Error: file.error})
	}
	return out
}

func (l *GooruLibrary) ImportUploadedFiles(ctx context.Context, files []StagedUpload, tags []string) (UploadImportResponse, error) {
	if err := ctx.Err(); err != nil {
		return UploadImportResponse{}, err
	}
	response := UploadImportResponse{Files: make([]UploadedFileDTO, 0, len(files))}
	hashes := make(map[string]string, len(files))
	importLocations := make([]types.LocationInfo, 0, len(files))
	responseIndexByPath := make(map[string]int, len(files))
	analysisPathByDestination := make(map[string]string, len(files))
	for _, file := range files {
		dto := UploadedFileDTO{Name: file.Name, Size: file.Size, TargetID: file.TargetID}
		if file.Status == "error" {
			dto.Status = "error"
			dto.Error = file.Error
			response.Files = append(response.Files, dto)
			continue
		}
		if file.Status == "skipped" {
			dto.Status = "skipped"
			response.Files = append(response.Files, dto)
			continue
		}
		analysisPath := file.AnalysisPath
		if analysisPath == "" {
			analysisPath = file.Path
		}
		info, status, err := l.client.GetFileInfoForFile(analysisPath, false)
		if err != nil {
			dto.Status = "error"
			dto.Error = err.Error()
			response.Files = append(response.Files, dto)
			continue
		}
		if _, ok := hashes[info.Hash]; ok {
			dto.Status = "duplicate_in_batch"
			removeRejectedStagedUpload(file)
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
			removeRejectedStagedUpload(file)
			response.Files = append(response.Files, dto)
			continue
		}
		dto.Status = "imported"
		response.Files = append(response.Files, dto)
		responseIndexByPath[file.Path] = len(response.Files) - 1
		analysisPathByDestination[file.Path] = analysisPath
		importLocations = append(importLocations, types.LocationInfo{
			Path:      file.Path,
			Hash:      info.Hash,
			Size:      info.Size,
			ModTime:   info.ModTime,
			Extension: filepath.Ext(file.Path),
		})
	}
	if len(importLocations) == 0 {
		return response, nil
	}
	failures := make(map[string]string)
	result, err := l.client.TagKnownFiles(importLocations, tags, func(filePath string, err error) {
		if err != nil {
			failures[filePath] = err.Error()
		}
	})
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
	l.cacheImportedMediaMetadata(ctx, importLocations, analysisPathByDestination)
	return response, nil
}

func removeRejectedStagedUpload(file StagedUpload) {
	_ = os.Remove(file.Path)
	if file.AnalysisPath != "" && file.AnalysisPath != file.Path {
		_ = os.Remove(file.AnalysisPath)
	}
}

func (l *GooruLibrary) cacheImportedMediaMetadata(ctx context.Context, files []types.LocationInfo, analysisPaths map[string]string) {
	provider := l.metadata
	if provider == nil {
		provider = BasicMediaMetadataProvider{}
	}
	for _, location := range files {
		if err := ctx.Err(); err != nil {
			return
		}
		file, err := l.client.GetFileInfoByPath(location.Path)
		if err != nil {
			continue
		}
		mediaType := mediaTypeForPath(file.Path)
		mediaKind := mediaKindForType(mediaType)
		analysisFile := file
		if analysisPath := analysisPaths[file.Path]; analysisPath != "" {
			analysisFile.Path = analysisPath
		}
		metadata, err := provider.Metadata(ctx, analysisFile, mediaType, mediaKind)
		if err != nil {
			continue
		}
		if metadata.ImageWidth == nil && metadata.ImageHeight == nil && metadata.VideoWidth == nil && metadata.VideoHeight == nil && metadata.VideoDuration == nil && metadata.FrameCount == nil {
			continue
		}
		_ = l.client.UpsertMediaMetadata(types.MediaMetadata{
			LocationID:      file.ID,
			MediaKind:       mediaKind,
			MimeType:        mediaType,
			ImageWidth:      metadata.ImageWidth,
			ImageHeight:     metadata.ImageHeight,
			VideoWidth:      metadata.VideoWidth,
			VideoHeight:     metadata.VideoHeight,
			DurationSeconds: metadata.VideoDuration,
			FrameCount:      metadata.FrameCount,
		})
	}
}
