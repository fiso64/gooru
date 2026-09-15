package serve

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	core "gooru.local/gooru"
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
	Name           string
	Path           string
	AnalysisPath   string
	Size           int64
	TargetID       string
	Status         string
	Error          string
	SourceModTime  time.Time
	AddedAt        time.Time
	ConflictPolicy string
}

type UploadTargetsResponse struct {
	Items []UploadTargetDTO `json:"items"`
}

type UploadTargetDTO struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	AddedAtStrategy string   `json:"added_at_strategy"`
	DefaultTags     []string `json:"default_tags"`
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
		strategy, err := uploadAddedAtStrategy("", target.AddedAtStrategy)
		if err != nil {
			continue
		}
		items = append(items, UploadTargetDTO{ID: target.ID, Name: target.Name, AddedAtStrategy: strategy, DefaultTags: append([]string(nil), target.DefaultTags...)})
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
	if err := r.Context().Err(); err != nil {
		writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
		return
	}
	// Real GooruLibrary uploads are selected into handleDurableUpload before
	// reaching this fallback. Importer-only doubles and embedders cannot safely
	// promise asynchronous recovery, so reject that capability before reading or
	// staging the body instead of creating a second in-memory upload work model.
	if PreferAsync(r) {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "asynchronous uploads require durable background operations", nil)
		return
	}
	tags, saved, err := s.stageMultipartUpload(r)
	if err != nil {
		writeMultipartUploadError(w, err)
		return
	}
	if err := query.ValidateTags(tags); err != nil {
		removeSavedUploads(saved)
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if len(saved) == 1 && saved[0].status == "error" && saved[0].error == errUploadTooLarge.Error() {
		fileErr := uploadFileError{name: saved[0].name, err: errUploadTooLarge}
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
		return
	}
	activated, err := activateSavedReplacements(saved)
	if err != nil {
		removeSavedUploads(saved)
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)
		return
	}
	response, err := importer.ImportUploadedFiles(r.Context(), stagedUploads(saved), tags)
	if err != nil {
		rollbackErr := rollbackSavedReplacements(activated)
		removeSavedUploads(saved)
		if rollbackErr != nil {
			err = fmt.Errorf("%w; replacement rollback failed: %v", err, rollbackErr)
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to import uploaded files", nil)
		return
	}
	if err := settleSavedReplacements(activated, response); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to finalize uploaded files", nil)
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
	name            string
	path            string
	destinationPath string
	size            int64
	targetID        string
	status          string
	error           string
	replace         bool
	sourceModTime   time.Time
	addedAt         time.Time
	conflictPolicy  string
	ownedFileInfo   os.FileInfo
}

var (
	errUploadTooLarge = errors.New("uploaded file exceeds max_file_size_bytes")
	errUploadConflict = errors.New("uploaded filename conflicts with an existing file")
)

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

func uploadAddedAtStrategy(requested string, fallback string) (string, error) {
	strategy := strings.TrimSpace(requested)
	if strategy == "" {
		strategy = strings.TrimSpace(fallback)
	}
	if strategy == "" {
		return "queue", nil
	}
	switch strategy {
	case "queue", "reverse_queue", "modtime":
		return strategy, nil
	default:
		if strings.TrimSpace(requested) != "" {
			return "", errors.New("added_at_strategy must be one of: queue, reverse_queue, modtime")
		}
		return "", errors.New("configured upload target added_at_strategy is invalid")
	}
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
			if len(files) > 1 {
				saved = append(saved, savedUpload{name: header.Filename, size: header.Size, targetID: target.ID, status: "error", error: err.Error()})
				continue
			}
			return nil, uploadFileError{name: header.Filename, err: err}
		}
		src, err := header.Open()
		if err != nil {
			fileErr := fmt.Errorf("failed to read uploaded file")
			if len(files) > 1 {
				saved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: fileErr.Error()})
				continue
			}
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: fileErr}
		}
		dst, path, tmpPath, skipped, err := createUploadDestination(target.Path, name, conflictPolicy)
		if err != nil {
			_ = src.Close()
			if len(files) > 1 && errors.Is(err, errUploadConflict) {
				saved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: err.Error()})
				continue
			}
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		if skipped {
			_ = src.Close()
			saved = append(saved, savedUpload{name: name, path: path, destinationPath: path, size: header.Size, targetID: target.ID, status: "skipped"})
			continue
		}
		size, copyErr := s.persistUploadedFile(dst, src)
		ownedFileInfo, statErr := dst.Stat()
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil || statErr != nil || closeErr != nil {
			_ = os.Remove(tmpPath)
			if errors.Is(copyErr, errUploadTooLarge) {
				saved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: copyErr.Error()})
				continue
			}
			removeSavedUploads(saved)
			if copyErr != nil {
				return nil, uploadFileError{name: name, err: copyErr}
			}
			if statErr != nil {
				return nil, uploadFileError{name: name, err: fmt.Errorf("failed to inspect uploaded file")}
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
				ownedFileInfo:   ownedFileInfo,
			})
			continue
		}
		if err := commitUploadDestination(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			if len(files) > 1 && errors.Is(err, errUploadConflict) {
				saved = append(saved, savedUpload{name: name, size: header.Size, targetID: target.ID, status: "error", error: err.Error()})
				continue
			}
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		saved = append(saved, savedUpload{name: filepath.Base(path), path: path, destinationPath: path, size: size, targetID: target.ID, ownedFileInfo: ownedFileInfo})
	}
	return saved, nil
}

func removeSavedUploads(files []savedUpload) {
	for _, file := range files {
		if file.status == "skipped" || file.status == "error" || file.ownedFileInfo == nil {
			continue
		}
		currentFileInfo, err := os.Stat(file.path)
		if err != nil || !os.SameFile(file.ownedFileInfo, currentFileInfo) {
			continue
		}
		_ = os.Remove(file.path)
	}
}

type activatedReplacement struct {
	finalPath            string
	backupPath           string
	noOriginalMarkerPath string
	hadOriginal          bool
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
				return nil, "", "", false, errUploadConflict
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
			return errUploadConflict
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
	replacement := activatedReplacement{
		finalPath:            finalPath,
		backupPath:           stagedPath + ".backup",
		noOriginalMarkerPath: stagedPath + ".no-original",
	}
	stagedExists, err := replacementPathExists(stagedPath)
	if err != nil {
		return activatedReplacement{}, fmt.Errorf("failed to inspect staged replacement")
	}
	finalExists, err := replacementPathExists(finalPath)
	if err != nil {
		return activatedReplacement{}, fmt.Errorf("failed to inspect existing file before replacement")
	}
	backupExists, err := replacementPathExists(replacement.backupPath)
	if err != nil {
		return activatedReplacement{}, fmt.Errorf("failed to inspect preserved file before replacement")
	}
	markerExists, err := replacementPathExists(replacement.noOriginalMarkerPath)
	if err != nil {
		return activatedReplacement{}, fmt.Errorf("failed to inspect replacement state marker")
	}
	if backupExists && markerExists {
		return activatedReplacement{}, errors.New("inconsistent staged replacement state")
	}
	if backupExists {
		replacement.hadOriginal = true
		return resumeReplacementActivation(replacement, stagedPath, stagedExists, finalExists)
	}
	if markerExists {
		return resumeReplacementActivation(replacement, stagedPath, stagedExists, finalExists)
	}
	if !stagedExists {
		return activatedReplacement{}, errors.New("staged replacement is missing")
	}
	if finalExists {
		if err := os.Rename(finalPath, replacement.backupPath); err != nil {
			return activatedReplacement{}, fmt.Errorf("failed to preserve existing file before replacement")
		}
		replacement.hadOriginal = true
	} else if err := createReplacementStateMarker(replacement.noOriginalMarkerPath); err != nil {
		return activatedReplacement{}, fmt.Errorf("failed to record replacement state")
	}
	if err := os.Rename(stagedPath, finalPath); err != nil {
		if replacement.hadOriginal {
			_ = os.Rename(replacement.backupPath, finalPath)
		} else {
			_ = os.Remove(replacement.noOriginalMarkerPath)
		}
		return activatedReplacement{}, fmt.Errorf("failed to store uploaded replacement")
	}
	return replacement, nil
}

func resumeReplacementActivation(replacement activatedReplacement, stagedPath string, stagedExists, finalExists bool) (activatedReplacement, error) {
	switch {
	case stagedExists && !finalExists:
		if err := os.Rename(stagedPath, replacement.finalPath); err != nil {
			return activatedReplacement{}, fmt.Errorf("failed to store uploaded replacement")
		}
		return replacement, nil
	case !stagedExists && finalExists:
		return replacement, nil
	default:
		return activatedReplacement{}, errors.New("inconsistent staged replacement state")
	}
}

func createReplacementStateMarker(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func replacementPathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func rollbackReplacement(replacement activatedReplacement) error {
	if replacement.hadOriginal {
		backupExists, err := replacementPathExists(replacement.backupPath)
		if err != nil {
			return fmt.Errorf("failed to inspect preserved original before replacement rollback")
		}
		if !backupExists {
			finalExists, err := replacementPathExists(replacement.finalPath)
			if err != nil {
				return fmt.Errorf("failed to inspect replacement rollback state")
			}
			if finalExists {
				return nil
			}
			return fmt.Errorf("failed to restore original file after replacement failure")
		}
	}
	if err := os.Remove(replacement.finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove rejected replacement")
	}
	if replacement.hadOriginal {
		if err := os.Rename(replacement.backupPath, replacement.finalPath); err != nil {
			return fmt.Errorf("failed to restore original file after replacement failure")
		}
	} else {
		_ = os.Remove(replacement.backupPath)
		_ = os.Remove(replacement.noOriginalMarkerPath)
	}
	return nil
}

func commitReplacement(replacement activatedReplacement) {
	_ = os.Remove(replacement.backupPath)
	_ = os.Remove(replacement.noOriginalMarkerPath)
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
		out = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, TargetID: file.targetID, Status: file.status, Error: file.error, SourceModTime: file.sourceModTime, AddedAt: file.addedAt, ConflictPolicy: file.conflictPolicy})
	}
	return out
}

type protectedUploadMove struct {
	storagePath string
	retryPath   string
}

func (l *GooruLibrary) ImportUploadedFiles(ctx context.Context, files []StagedUpload, tags []string) (UploadImportResponse, error) {
	return l.importUploadedFiles(ctx, files, tags, backgroundUploadImportState{}, nil)
}

func (l *GooruLibrary) setBackgroundUploadImportCheckpoint(state backgroundUploadImportState, checkpoint backgroundUploadCheckpoint) error {
	if state.taskID != "" {
		if err := l.client.SetBackgroundTaskCheckpoint(state.taskID, checkpoint); err != nil {
			return err
		}
	}
	if state.operationID != "" {
		return l.client.SetBackgroundOperationCheckpoint(state.operationID, checkpoint)
	}
	return nil
}

func (l *GooruLibrary) importUploadedFiles(ctx context.Context, files []StagedUpload, tags []string, state backgroundUploadImportState, activated []activatedSavedReplacement) (UploadImportResponse, error) {
	if err := ctx.Err(); err != nil {
		return UploadImportResponse{}, err
	}
	response := UploadImportResponse{Files: make([]UploadedFileDTO, 0, len(files))}
	hashes := make(map[string]string, len(files))
	importLocations := make([]types.LocationInfo, 0, len(files))
	responseIndexByPath := make(map[string]int, len(files))
	analysisPathByDestination := make(map[string]string, len(files))
	opaqueStorageByLogical := make(map[string]protectedUploadMove, len(files))
	analyses, analysisErr := l.analyzeUploadedFiles(ctx, files, func(completed, completedPrefix int) error {
		if (state.operationID == "" && state.taskID == "") || !shouldPersistUploadProgress(completed, len(files)) {
			return nil
		}
		return l.setBackgroundUploadImportCheckpoint(state, backgroundUploadActivatedCheckpoint(activated, len(files), completed, completedPrefix))
	})
	if analysisErr != nil {
		return UploadImportResponse{}, analysisErr
	}
	for index, file := range files {
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
		analysis := analyses[index]
		analysisPath := analysis.AnalysisPath
		if analysis.Err != nil {
			dto.Status = "error"
			dto.Error = analysis.Err.Error()
			response.Files = append(response.Files, dto)
			continue
		}
		info, status := analysis.Info, analysis.Status
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
			trackedAtPath := status == types.StatusOK && !(l.encryption.Enabled && IsManagedUploadPath(l.managedTargets, file.Path))
			discardDuplicateUpload(file, trackedAtPath)
			response.Files = append(response.Files, dto)
			continue
		}
		retryPath := file.Path
		if file.ConflictPolicy == "rename" && l.encryption.Enabled && IsManagedUploadPath(l.managedTargets, file.Path) {
			resolvedPath, err := l.resolveProtectedUploadRename(file.Path)
			if err != nil {
				dto.Status = "error"
				dto.Error = err.Error()
				removeRejectedStagedUpload(file)
				response.Files = append(response.Files, dto)
				continue
			}
			file.Path = resolvedPath
			dto.Name = filepath.Base(resolvedPath)
		}
		storagePath := ""
		if l.encryption.Enabled && IsManagedUploadPath(l.managedTargets, file.Path) {
			storagePath, err = moveManagedFileToOpaqueStorage(file.Path, info.Hash, l.encryption.Key)
			if err != nil {
				dto.Status = "error"
				dto.Error = err.Error()
				response.Files = append(response.Files, dto)
				continue
			}
			opaqueStorageByLogical[file.Path] = protectedUploadMove{storagePath: storagePath, retryPath: retryPath}
			analysisPath = storagePath
		}
		dto.Status = "imported"
		response.Files = append(response.Files, dto)
		responseIndexByPath[file.Path] = len(response.Files) - 1
		analysisPathByDestination[file.Path] = analysisPath
		addedAt := int64(0)
		if !file.AddedAt.IsZero() {
			addedAt = file.AddedAt.Unix()
		}
		importLocations = append(importLocations, types.LocationInfo{
			Path:        file.Path,
			StoragePath: storagePath,
			Hash:        info.Hash,
			Size:        info.Size,
			ModTime:     info.ModTime,
			AddedAt:     addedAt,
			Extension:   filepath.Ext(file.Path),
		})
	}
	if len(importLocations) == 0 {
		return response, nil
	}
	backgroundTasks := make([]core.BackgroundTaskRequest, 0, len(importLocations))
	if l.backgroundTasks != nil {
		for _, location := range importLocations {
			backgroundTasks = append(backgroundTasks, l.backgroundTasks(location)...)
		}
	}
	failures := make(map[string]string)
	progress := func(filePath string, err error) {
		if err != nil {
			failures[filePath] = err.Error()
		}
	}
	var result types.TagOperationResult
	var err error
	if state.operationID == "" && state.taskID == "" {
		result, err = l.client.TagKnownFilesWithBackgroundTasks(importLocations, tags, backgroundTasks, progress)
	} else {
		result, err = l.client.TagKnownFilesWithBackgroundTasksAndOperationState(importLocations, tags, backgroundTasks, func(affectedCount int) (core.BackgroundOperationTransactionState, error) {
			response.AffectedCount = affectedCount
			checkpoint := backgroundUploadImportedCheckpoint(activated, response)
			return core.BackgroundOperationTransactionState{OperationID: state.operationID, TaskID: state.taskID, Checkpoint: checkpoint, Result: response}, nil
		}, progress)
	}
	if err != nil {
		var restoreErr error
		for _, moved := range opaqueStorageByLogical {
			if moveErr := commitUploadDestination(moved.storagePath, moved.retryPath); moveErr != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("restore protected upload %q: %w", moved.retryPath, moveErr))
			}
		}
		if restoreErr != nil {
			return UploadImportResponse{}, errors.Join(err, restoreErr)
		}
		return UploadImportResponse{}, err
	}
	for path, message := range failures {
		if i, ok := responseIndexByPath[path]; ok {
			response.Files[i].Status = "error"
			response.Files[i].Error = message
			if moved, ok := opaqueStorageByLogical[path]; ok {
				_ = os.Remove(moved.storagePath)
			} else {
				_ = os.Remove(path)
			}
		}
	}
	response.AffectedCount = result.AffectedCount
	response.Notifications = notificationDTOs(result.Notifications)
	l.cacheImportedMediaMetadata(ctx, importLocations, analysisPathByDestination)
	return response, nil
}

func (l *GooruLibrary) resolveProtectedUploadRename(path string) (string, error) {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	if base == "" {
		base = "upload"
	}
	dir := filepath.Dir(path)
	for i := 0; i < 10_000; i++ {
		name := filepath.Base(path)
		if i > 0 {
			name = fmt.Sprintf("%s-%d%s", base, i, ext)
		}
		candidate := filepath.Join(dir, name)
		_, err := l.client.GetFileInfoByPath(candidate)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("check protected upload name: %w", err)
		}
		if candidate == path {
			return path, nil
		}
		if err := commitUploadDestination(path, candidate); err != nil {
			if errors.Is(err, errUploadConflict) {
				continue
			}
			return "", err
		}
		return candidate, nil
	}
	return "", errors.New("could not choose a non-conflicting protected upload filename")
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
		metadata, err := l.importedMediaMetadata(ctx, provider, file, analysisPaths[file.Path], mediaType, mediaKind)
		if err != nil {
			continue
		}
		if metadata.ImageWidth == nil && metadata.ImageHeight == nil && metadata.VideoWidth == nil && metadata.VideoHeight == nil && metadata.VideoDuration == nil && metadata.FrameCount == nil && metadata.PageCount == nil {
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
			PageCount:       metadata.PageCount,
		})
	}
}
