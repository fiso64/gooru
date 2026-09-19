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
	ID       string `json:"id,omitempty"`
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
	AddedOrder     int64
	ConflictPolicy string
	Tags           *[]string
	OwnershipPath  string
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
	defer releaseSavedUploadOwnership(saved)
	if err := validateUploadTags(tags, saved); err != nil {
		removeSavedUploads(saved)
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	if len(saved) == 1 && saved[0].status == "error" && saved[0].error == errUploadTooLarge.Error() {
		fileErr := uploadFileError{name: saved[0].name, err: errUploadTooLarge}
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", fileErr.Error(), uploadErrorDetails(fileErr))
		return
	}
	response, err := importer.ImportUploadedFiles(r.Context(), stagedUploads(saved), tags)
	if err != nil {
		removeSavedUploads(saved)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			writeError(w, http.StatusRequestTimeout, "request_canceled", "request was canceled", nil)
			return
		}
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
	name            string
	path            string
	destinationPath string
	size            int64
	targetID        string
	status          string
	error           string
	sourceModTime   time.Time
	addedAt         time.Time
	addedOrder      int64
	conflictPolicy  string
	tags            *[]string
	ownershipPath   string
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

func uploadConflictPolicy(requested string) (string, error) {
	switch strings.TrimSpace(requested) {
	case "", "rename":
		return "rename", nil
	case "error":
		return "error", nil
	default:
		return "", errors.New("conflict_policy must be one of: rename, error")
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
		dst, path, tmpPath, err := createUploadDestination(target.Path, name, conflictPolicy)
		if err != nil {
			_ = src.Close()
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		size, copyErr := s.persistUploadedFile(dst, src)
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
		if err := commitUploadDestinationWithOwnership(tmpPath, path); err != nil {
			_ = os.Remove(tmpPath)
			removeSavedUploads(saved)
			return nil, uploadFileError{name: name, err: err}
		}
		saved = append(saved, savedUpload{name: filepath.Base(path), path: path, destinationPath: path, size: size, targetID: target.ID, ownershipPath: tmpPath})
	}
	return saved, nil
}

func removeSavedUploads(files []savedUpload) {
	for _, file := range files {
		if file.status == "skipped" || file.status == "error" {
			continue
		}
		if file.ownershipPath == "" {
			continue
		}
		ownedFileInfo, err := os.Stat(file.ownershipPath)
		if err == nil {
			if currentFileInfo, statErr := os.Stat(file.path); statErr == nil && os.SameFile(ownedFileInfo, currentFileInfo) {
				_ = os.Remove(file.path)
			}
		}
		_ = os.Remove(file.ownershipPath)
	}
}

func releaseSavedUploadOwnership(files []savedUpload) {
	for _, file := range files {
		if file.ownershipPath != "" {
			_ = os.Remove(file.ownershipPath)
		}
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

func createUploadDestination(dir string, name string, conflictPolicy string) (*os.File, string, string, error) {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(filepath.Base(name), ext)
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
			if conflictPolicy == "error" {
				return nil, "", "", errUploadConflict
			}
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
	if err := commitUploadDestinationWithOwnership(tmpPath, finalPath); err != nil {
		return err
	}
	if err := os.Remove(tmpPath); err != nil {
		_ = os.Remove(finalPath)
		return fmt.Errorf("failed to store uploaded file")
	}
	return nil
}

func commitUploadDestinationWithOwnership(tmpPath string, finalPath string) error {
	if err := os.Link(tmpPath, finalPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errUploadConflict
		}
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

func attachUploadItemTags(files []savedUpload, values []string) error {
	if len(values) == 0 {
		return nil
	}
	if len(values) != len(files) {
		return multipartUploadError{message: fmt.Sprintf("item_tags count %d does not match file count %d", len(values), len(files)), err: errors.New("item_tags must align with uploaded files")}
	}
	for index, value := range values {
		tags := parseUploadTags([]string{value})
		files[index].tags = &tags
	}
	return nil
}

func validateUploadTags(fallback []string, files []savedUpload) error {
	if err := query.ValidateTags(fallback); err != nil {
		return err
	}
	for index, file := range files {
		if file.tags == nil {
			continue
		}
		if err := query.ValidateTags(*file.tags); err != nil {
			return fmt.Errorf("invalid tags for file %d: %w", index, err)
		}
	}
	return nil
}

func cloneUploadTags(tags *[]string) *[]string {
	if tags == nil {
		return nil
	}
	copyTags := make([]string, len(*tags))
	copy(copyTags, *tags)
	return &copyTags
}

func resolvedUploadTags(tags *[]string, fallback []string) []string {
	if tags != nil {
		return *tags
	}
	return fallback
}

func mergeUploadTags(existing, additions []string) []string {
	if len(additions) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, tag := range existing {
		seen[tag] = struct{}{}
	}
	for _, tag := range additions {
		if _, ok := seen[tag]; ok {
			continue
		}
		existing = append(existing, tag)
		seen[tag] = struct{}{}
	}
	return existing
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
		out = append(out, StagedUpload{Name: file.name, Path: path, AnalysisPath: path, Size: file.size, TargetID: file.targetID, Status: file.status, Error: file.error, SourceModTime: file.sourceModTime, AddedAt: file.addedAt, AddedOrder: file.addedOrder, ConflictPolicy: file.conflictPolicy, Tags: cloneUploadTags(file.tags), OwnershipPath: file.ownershipPath})
	}
	return out
}

type protectedUploadMove struct {
	storagePath string
	retryPath   string
}

func (l *GooruLibrary) ImportUploadedFiles(ctx context.Context, files []StagedUpload, tags []string) (UploadImportResponse, error) {
	return l.importUploadedFiles(ctx, files, tags, backgroundUploadImportState{})
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

func (l *GooruLibrary) populateUploadFileIDs(response *UploadImportResponse, files []StagedUpload) error {
	if response == nil {
		return nil
	}
	paths := make([]string, 0, len(response.Files))
	pathByResponseIndex := make(map[int]string)
	for i := range response.Files {
		dto := &response.Files[i]
		if dto.ID != "" || dto.Status != "imported" {
			continue
		}
		if i >= len(files) {
			return fmt.Errorf("resolve imported upload identity %q: source row is missing", dto.Name)
		}
		path := files[i].Path
		if dto.Name != "" && filepath.Base(path) != dto.Name {
			path = filepath.Join(filepath.Dir(path), dto.Name)
		}
		paths = append(paths, path)
		pathByResponseIndex[i] = path
	}
	if len(paths) == 0 {
		return nil
	}
	infosByPath, err := l.client.GetFileInfosByPaths(paths)
	if err != nil {
		return fmt.Errorf("resolve imported upload identities: %w", err)
	}
	for i := range response.Files {
		path, ok := pathByResponseIndex[i]
		if !ok {
			continue
		}
		dto := &response.Files[i]
		info, ok := infosByPath[path]
		if !ok {
			return fmt.Errorf("resolve imported upload identity %q: tracked path is unavailable", dto.Name)
		}
		dto.ID = info.PublicID
		if dto.ID == "" {
			return fmt.Errorf("resolve imported upload identity %q: public id is unavailable", dto.Name)
		}
	}
	return nil
}

func (l *GooruLibrary) importUploadedFiles(ctx context.Context, files []StagedUpload, tags []string, state backgroundUploadImportState) (UploadImportResponse, error) {
	if err := ctx.Err(); err != nil {
		return UploadImportResponse{}, err
	}
	response := UploadImportResponse{Files: make([]UploadedFileDTO, 0, len(files))}
	firstResponseIndexByHash := make(map[string]int, len(files))
	duplicateCanonicalResponseIndex := make(map[int]int)
	importLocations := make([]types.LocationInfo, 0, len(files))
	addedOrderByPath := make(map[string]int64, len(files))
	responseIndexByPath := make(map[string]int, len(files))
	analysisPathByDestination := make(map[string]string, len(files))
	ownershipPathByDestination := make(map[string]string, len(files))
	opaqueStorageByLogical := make(map[string]protectedUploadMove, len(files))
	duplicateExistingDiscards := make([]struct {
		file          StagedUpload
		trackedAtPath bool
	}, 0, len(files))
	analyses, analysisErr := l.analyzeUploadedFiles(ctx, files, func(completed, completedPrefix int) error {
		if (state.operationID == "" && state.taskID == "") || !shouldPersistUploadProgress(completed, len(files)) {
			return nil
		}
		return l.setBackgroundUploadImportCheckpoint(state, backgroundUploadActivatedCheckpoint(len(files), completed, completedPrefix))
	})
	if analysisErr != nil {
		return UploadImportResponse{}, analysisErr
	}
	tagsByHash := make(map[string][]string, len(files))
	identityHashes := make([]string, 0, len(files))
	seenIdentityHashes := make(map[string]struct{}, len(files))
	for index, file := range files {
		if file.Status == "error" || file.Status == "skipped" || index >= len(analyses) || analyses[index].Err != nil || analyses[index].Info.Hash == "" {
			continue
		}
		hash := analyses[index].Info.Hash
		tagsByHash[hash] = mergeUploadTags(tagsByHash[hash], resolvedUploadTags(file.Tags, tags))
		if _, seen := seenIdentityHashes[hash]; !seen {
			seenIdentityHashes[hash] = struct{}{}
			identityHashes = append(identityHashes, hash)
		}
	}
	trackedByHash, lookupErr := l.client.GetFileInfosByContentHashes(identityHashes)
	if lookupErr != nil {
		return UploadImportResponse{}, fmt.Errorf("resolve existing upload identities: %w", lookupErr)
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
		info := analysis.Info
		if canonicalIndex, ok := firstResponseIndexByHash[info.Hash]; ok {
			dto.Status = "duplicate_in_batch"
			dto.ID = response.Files[canonicalIndex].ID
			if dto.ID == "" {
				duplicateCanonicalResponseIndex[len(response.Files)] = canonicalIndex
			}
			removeRejectedStagedUpload(file)
			response.Files = append(response.Files, dto)
			continue
		}
		firstResponseIndexByHash[info.Hash] = len(response.Files)
		if existing, exists := trackedByHash[info.Hash]; exists {
			dto.ID = existing.PublicID
			if dto.ID == "" {
				return UploadImportResponse{}, fmt.Errorf("resolve duplicate upload identity %q: public id is unavailable", file.Name)
			}
			dto.Status = "duplicate_existing"
			trackedAtPath := analysis.Status == types.StatusOK && !(l.encryption.Enabled && IsManagedUploadPath(l.managedTargets, file.Path))
			duplicateExistingDiscards = append(duplicateExistingDiscards, struct {
				file          StagedUpload
				trackedAtPath bool
			}{file: file, trackedAtPath: trackedAtPath})
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
			movedStoragePath, moveErr := moveManagedFileToOpaqueStorage(file.Path, info.Hash, l.encryption.Key)
			if moveErr != nil {
				dto.Status = "error"
				dto.Error = moveErr.Error()
				response.Files = append(response.Files, dto)
				continue
			}
			storagePath = movedStoragePath
			opaqueStorageByLogical[file.Path] = protectedUploadMove{storagePath: storagePath, retryPath: retryPath}
			analysisPath = storagePath
		}
		dto.Status = "imported"
		response.Files = append(response.Files, dto)
		responseIndexByPath[file.Path] = len(response.Files) - 1
		analysisPathByDestination[file.Path] = analysisPath
		ownershipPathByDestination[file.Path] = file.OwnershipPath
		addedAt := int64(0)
		if !file.AddedAt.IsZero() {
			addedAt = file.AddedAt.UnixMilli()
		}
		addedOrderByPath[file.Path] = file.AddedOrder
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
		result, err = l.client.TagKnownFilesWithBackgroundTasksByHashTagsWithAddedOrder(importLocations, addedOrderByPath, tagsByHash, backgroundTasks, progress)
	} else {
		result, err = l.client.TagKnownFilesWithBackgroundTasksByHashTagsAndOperationStateWithAddedOrder(importLocations, addedOrderByPath, tagsByHash, backgroundTasks, func(affectedCount int) (core.BackgroundOperationTransactionState, error) {
			response.AffectedCount = affectedCount
			checkpoint := backgroundUploadImportedCheckpoint(response)
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
	for _, duplicate := range duplicateExistingDiscards {
		discardDuplicateUpload(duplicate.file, duplicate.trackedAtPath)
	}
	for path, message := range failures {
		if i, ok := responseIndexByPath[path]; ok {
			response.Files[i].Status = "error"
			response.Files[i].Error = message
			if moved, ok := opaqueStorageByLogical[path]; ok {
				removeStagedUploadPath(moved.storagePath, ownershipPathByDestination[path])
			} else {
				removeStagedUploadPath(path, ownershipPathByDestination[path])
			}
		}
	}
	response.AffectedCount = result.AffectedCount
	response.Notifications = notificationDTOs(result.Notifications)
	if err := l.populateUploadFileIDs(&response, files); err != nil {
		return UploadImportResponse{}, err
	}
	for duplicateIndex, canonicalIndex := range duplicateCanonicalResponseIndex {
		if duplicateIndex >= len(response.Files) || canonicalIndex >= len(response.Files) {
			return UploadImportResponse{}, errors.New("resolve same-batch duplicate upload identity: response row is missing")
		}
		if canonicalID := response.Files[canonicalIndex].ID; canonicalID != "" {
			response.Files[duplicateIndex].ID = canonicalID
		}
	}
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
	removeStagedUploadPath(file.Path, file.OwnershipPath)
	if file.AnalysisPath != "" && file.AnalysisPath != file.Path {
		removeStagedUploadPath(file.AnalysisPath, file.OwnershipPath)
	}
}

func removeStagedUploadPath(path, ownershipPath string) {
	if path == "" {
		return
	}
	if ownershipPath == "" {
		_ = os.Remove(path)
		return
	}
	ownedInfo, err := os.Stat(ownershipPath)
	if err != nil {
		return
	}
	currentInfo, err := os.Stat(path)
	if err != nil || !os.SameFile(ownedInfo, currentInfo) {
		return
	}
	_ = os.Remove(path)
}

func (l *GooruLibrary) cacheImportedMediaMetadata(ctx context.Context, files []types.LocationInfo, analysisPaths map[string]string) {
	if err := ctx.Err(); err != nil || len(files) == 0 {
		return
	}
	paths := make([]string, len(files))
	for i, location := range files {
		paths[i] = location.Path
	}
	filesByPath, err := l.client.GetFileInfosByPaths(paths)
	if err != nil {
		return
	}

	provider := l.metadata
	if provider == nil {
		provider = BasicMediaMetadataProvider{}
	}
	metadataRows := make([]types.MediaMetadata, 0, len(files))
	persistMetadata := func() {
		if len(metadataRows) > 0 {
			_ = l.client.BatchUpsertMediaMetadata(metadataRows)
		}
	}
	for _, location := range files {
		if err := ctx.Err(); err != nil {
			persistMetadata()
			return
		}
		file, ok := filesByPath[location.Path]
		if !ok {
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
		metadataRows = append(metadataRows, types.MediaMetadata{
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
	persistMetadata()
}
