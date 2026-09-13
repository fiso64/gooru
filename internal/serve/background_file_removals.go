package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/types"
)

const (
	backgroundFileRemovalTaskKind      = "files.remove"
	backgroundFileRemovalResourceClass = "storage"
	backgroundFileRemovalBatchVersion  = 2
)

// backgroundFileRemovalInput is the legacy one-file payload. Keep decoding it
// so queued work from an older binary can finish after an upgrade.
type backgroundFileRemovalInput struct {
	Mode         string `json:"mode"`
	PublicID     string `json:"public_id"`
	OriginalPath string `json:"original_path,omitempty"`
	StagingPath  string `json:"staging_path,omitempty"`
}

type backgroundFileRemovalBatchInput struct {
	Version int                              `json:"version"`
	Mode    string                           `json:"mode"`
	Files   []backgroundFileRemovalBatchFile `json:"files"`
}

type backgroundFileRemovalBatchFile struct {
	PublicID     string `json:"public_id"`
	OriginalPath string `json:"original_path,omitempty"`
	StagingPath  string `json:"staging_path,omitempty"`
}

func (s *Server) backgroundFileRemovalBatchTask(mode string, files []types.FileInfo) (core.BackgroundTaskRequest, error) {
	if len(files) == 0 {
		return core.BackgroundTaskRequest{}, errors.New("file removal batch is empty")
	}
	input := backgroundFileRemovalBatchInput{
		Version: backgroundFileRemovalBatchVersion,
		Mode:    mode,
		Files:   make([]backgroundFileRemovalBatchFile, 0, len(files)),
	}
	stagingToken := ""
	if mode == "delete" {
		var err error
		stagingToken, err = newFileRemovalToken()
		if err != nil {
			return core.BackgroundTaskRequest{}, err
		}
	}
	for index, file := range files {
		item := backgroundFileRemovalBatchFile{PublicID: s.publicFileID(file)}
		if item.PublicID == "" {
			return core.BackgroundTaskRequest{}, errors.New("file removal batch contains a file without public identity")
		}
		if mode == "delete" {
			managedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))
			if !ok {
				return core.BackgroundTaskRequest{}, ErrFileNotManaged
			}
			item.OriginalPath = managedPath
			item.StagingPath = filepath.Join(
				filepath.Dir(managedPath),
				fmt.Sprintf(".gooru-delete-%s-%06d", stagingToken, index),
			)
		}
		input.Files = append(input.Files, item)
	}
	if mode != "delete" && mode != "untrack" {
		return core.BackgroundTaskRequest{}, fmt.Errorf("file removal batch has invalid mode %q", mode)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return core.BackgroundTaskRequest{}, fmt.Errorf("encode file removal batch task: %w", err)
	}
	return core.BackgroundTaskRequest{
		DedupeKey:     "batch",
		Kind:          backgroundFileRemovalTaskKind,
		SubjectKind:   "file_batch",
		SubjectID:     "selection",
		InputKey:      string(encoded),
		ResourceClass: backgroundFileRemovalResourceClass,
		MaxAttempts:   5,
	}, nil
}

func newFileRemovalToken() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate file removal staging token: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}

func (s *Server) backgroundFileRemovalHandler(ctx context.Context, task core.BackgroundTask) error {
	var envelope struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(task.InputKey), &envelope); err != nil {
		return fmt.Errorf("decode file removal task envelope: %w", err)
	}
	if envelope.Version == backgroundFileRemovalBatchVersion {
		var input backgroundFileRemovalBatchInput
		if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
			return fmt.Errorf("decode file removal batch task: %w", err)
		}
		if task.SubjectKind != "file_batch" || task.SubjectID != "selection" || len(input.Files) == 0 {
			return errors.New("file removal batch task has invalid identity")
		}
		return s.runBackgroundFileRemovalBatch(ctx, input)
	}

	var input backgroundFileRemovalInput
	if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
		return fmt.Errorf("decode file removal task: %w", err)
	}
	if task.SubjectKind != "file" || task.SubjectID == "" || task.SubjectID != input.PublicID {
		return errors.New("file removal task has invalid file identity")
	}
	switch input.Mode {
	case "untrack":
		_, err := s.deleteFileByPublicID(ctx, input.PublicID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	case "delete":
		return s.resumeManagedFileDeletion(ctx, input)
	default:
		return fmt.Errorf("file removal task has invalid mode %q", input.Mode)
	}
}

func (s *Server) runBackgroundFileRemovalBatch(ctx context.Context, input backgroundFileRemovalBatchInput) error {
	publicIDs := make([]string, 0, len(input.Files))
	for _, file := range input.Files {
		if file.PublicID == "" {
			return errors.New("file removal batch contains a file without public identity")
		}
		publicIDs = append(publicIDs, file.PublicID)
	}

	switch input.Mode {
	case "untrack":
		_, err := s.deleteFilesByPublicIDs(ctx, publicIDs)
		return err
	case "delete":
		return s.resumeManagedFileDeletionBatch(ctx, input.Files, publicIDs)
	default:
		return fmt.Errorf("file removal batch task has invalid mode %q", input.Mode)
	}
}

func (s *Server) resumeManagedFileDeletionBatch(ctx context.Context, files []backgroundFileRemovalBatchFile, publicIDs []string) error {
	stagedFiles := make([]*flatStagedFileDeletion, 0, len(files))
	for _, input := range files {
		if err := ctx.Err(); err != nil {
			return rollbackFlatStagedFiles(stagedFiles, err)
		}
		staged, err := persistedFlatStagedFileDeletion(input.OriginalPath, input.StagingPath)
		if err != nil {
			return rollbackFlatStagedFiles(stagedFiles, err)
		}
		if err := s.validatePersistedDeletionParent(staged.originalPath); err != nil {
			return rollbackFlatStagedFiles(stagedFiles, err)
		}
		stagedExists, err := safeStagedDeletionEntryExists(staged.stagedPath)
		if err != nil {
			return rollbackFlatStagedFiles(stagedFiles, err)
		}
		if stagedExists {
			if err := s.validatePersistedStagedDeletionRetry(ctx, input.PublicID, staged.originalPath); err != nil {
				return rollbackFlatStagedFiles(stagedFiles, err)
			}
		} else {
			file, err := s.getFileByPublicID(ctx, input.PublicID)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return rollbackFlatStagedFiles(stagedFiles, err)
			}
			managedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))
			if !ok {
				return rollbackFlatStagedFiles(stagedFiles, ErrFileNotManaged)
			}
			if filepath.Clean(managedPath) != staged.originalPath {
				return rollbackFlatStagedFiles(stagedFiles, errors.New("managed file path changed after deletion was queued"))
			}
			if err := staged.stage(); err != nil {
				return rollbackFlatStagedFiles(stagedFiles, err)
			}
		}
		stagedFiles = append(stagedFiles, staged)
	}

	if _, err := s.deleteFilesByPublicIDs(ctx, publicIDs); err != nil {
		return rollbackFlatStagedFiles(stagedFiles, fmt.Errorf("delete library locations: %w", err))
	}

	for _, staged := range stagedFiles {
		if err := staged.commitMissingOK(); err != nil {
			// The database commit is the point of no return. A durable retry will
			// find the original public IDs absent and continue cleaning the exact
			// persisted hidden staging paths without touching any replacement file.
			return fmt.Errorf("remove staged file: %w", err)
		}
	}
	return nil
}

func (s *Server) validatePersistedStagedDeletionRetry(ctx context.Context, publicID, originalPath string) error {
	originalExists, err := pathExists(originalPath)
	if err != nil {
		return fmt.Errorf("check original path before staged deletion retry: %w", err)
	}
	if !originalExists {
		return nil
	}

	_, err = s.getFileByPublicID(ctx, publicID)
	if errors.Is(err, ErrNotFound) {
		// The database delete already committed. The occupied original path is a
		// replacement, so leave it alone and finish removing the staged original.
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve file before staged deletion retry: %w", err)
	}
	return errors.New("cannot resume staged file deletion because original path is occupied before database removal")
}

func rollbackFlatStagedFiles(stagedFiles []*flatStagedFileDeletion, cause error) error {
	var rollbackErr error
	for index := len(stagedFiles) - 1; index >= 0; index-- {
		if err := stagedFiles[index].rollbackMissingOK(); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	if rollbackErr != nil {
		return errors.Join(cause, fmt.Errorf("restore staged files: %w", rollbackErr))
	}
	return cause
}

type flatStagedFileDeletion struct {
	originalPath string
	stagedPath   string
}

func persistedFlatStagedFileDeletion(originalPath, stagedPath string) (*flatStagedFileDeletion, error) {
	originalPath = filepath.Clean(strings.TrimSpace(originalPath))
	stagedPath = filepath.Clean(strings.TrimSpace(stagedPath))
	if originalPath == "." || stagedPath == "." || originalPath == "" || stagedPath == "" {
		return nil, errors.New("file removal batch is missing persisted paths")
	}
	if filepath.Dir(stagedPath) != filepath.Dir(originalPath) || !strings.HasPrefix(filepath.Base(stagedPath), ".gooru-delete-") {
		return nil, errors.New("file removal batch has invalid staging path")
	}
	return &flatStagedFileDeletion{originalPath: originalPath, stagedPath: stagedPath}, nil
}

func (d *flatStagedFileDeletion) stage() error {
	if err := os.Rename(d.originalPath, d.stagedPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stage file deletion: %w", err)
	}
	return nil
}

func (d *flatStagedFileDeletion) rollbackMissingOK() error {
	stagedExists, err := safeStagedDeletionEntryExists(d.stagedPath)
	if err != nil || !stagedExists {
		return err
	}
	originalExists, err := pathExists(d.originalPath)
	if err != nil {
		return err
	}
	if originalExists {
		return errors.New("cannot restore staged file because original path is occupied")
	}
	return os.Rename(d.stagedPath, d.originalPath)
}

func (d *flatStagedFileDeletion) commitMissingOK() error {
	stagedExists, err := safeStagedDeletionEntryExists(d.stagedPath)
	if err != nil || !stagedExists {
		return err
	}
	if err := os.Remove(d.stagedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *Server) resumeManagedFileDeletion(ctx context.Context, input backgroundFileRemovalInput) error {
	staged, err := persistedStagedFileDeletion(input.OriginalPath, input.StagingPath)
	if err != nil {
		return err
	}
	if err := s.validatePersistedDeletionParent(staged.originalPath); err != nil {
		return err
	}
	if _, err := validateDeletionStagingDirectory(staged.stagingDir); err != nil {
		return err
	}
	stagedExists, err := safeStagedDeletionEntryExists(staged.stagedPath)
	if err != nil {
		return err
	}
	if stagedExists {
		if err := s.validatePersistedStagedDeletionRetry(ctx, input.PublicID, staged.originalPath); err != nil {
			return err
		}
	} else {
		file, err := s.getFileByPublicID(ctx, input.PublicID)
		if errors.Is(err, ErrNotFound) {
			return staged.commitMissingOK()
		}
		if err != nil {
			return err
		}
		managedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))
		if !ok {
			return ErrFileNotManaged
		}
		if filepath.Clean(managedPath) != staged.originalPath {
			return errors.New("managed file path changed after deletion was queued")
		}
		if err := staged.stage(); err != nil {
			return err
		}
	}

	deleted, deleteErr := s.deleteFileByPublicID(ctx, input.PublicID)
	if deleteErr != nil && !errors.Is(deleteErr, ErrNotFound) {
		if rollbackErr := staged.rollbackMissingOK(); rollbackErr != nil {
			return fmt.Errorf("delete library location: %v; restore file: %w", deleteErr, rollbackErr)
		}
		return deleteErr
	}
	if !deleted && deleteErr == nil {
		deleteErr = ErrNotFound
	}
	// Once the database location is absent, never restore the staged file.
	// A crash or cleanup failure here is recovered by the durable retry using
	// the persisted staging path even though the public ID no longer resolves.
	if err := staged.commitMissingOK(); err != nil {
		return fmt.Errorf("remove staged file: %w", err)
	}
	return nil
}

func persistedStagedFileDeletion(originalPath, stagedPath string) (*stagedFileDeletion, error) {
	originalPath = filepath.Clean(strings.TrimSpace(originalPath))
	stagedPath = filepath.Clean(strings.TrimSpace(stagedPath))
	if originalPath == "." || stagedPath == "." || originalPath == "" || stagedPath == "" {
		return nil, errors.New("file removal task is missing persisted paths")
	}
	stagingDir := filepath.Dir(stagedPath)
	if filepath.Dir(stagingDir) != filepath.Dir(originalPath) || !strings.HasPrefix(filepath.Base(stagingDir), ".gooru-delete-") || filepath.Base(stagedPath) != filepath.Base(originalPath) {
		return nil, errors.New("file removal task has invalid staging path")
	}
	return &stagedFileDeletion{originalPath: originalPath, stagedPath: stagedPath, stagingDir: stagingDir}, nil
}

func (d *stagedFileDeletion) stage() error {
	if err := ensureDeletionStagingDirectory(d.stagingDir); err != nil {
		return err
	}
	if err := os.Rename(d.originalPath, d.stagedPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stage file deletion: %w", err)
	}
	return nil
}

func (d *stagedFileDeletion) rollbackMissingOK() error {
	stagingDirExists, err := validateDeletionStagingDirectory(d.stagingDir)
	if err != nil || !stagingDirExists {
		return err
	}
	stagedExists, err := safeStagedDeletionEntryExists(d.stagedPath)
	if err != nil || !stagedExists {
		return err
	}
	originalExists, err := pathExists(d.originalPath)
	if err != nil {
		return err
	}
	if originalExists {
		return errors.New("cannot restore staged file because original path is occupied")
	}
	if err := os.Rename(d.stagedPath, d.originalPath); err != nil {
		return err
	}
	_ = os.Remove(d.stagingDir)
	return nil
}

func (d *stagedFileDeletion) commitMissingOK() error {
	stagingDirExists, err := validateDeletionStagingDirectory(d.stagingDir)
	if err != nil || !stagingDirExists {
		return err
	}
	stagedExists, err := safeStagedDeletionEntryExists(d.stagedPath)
	if err != nil {
		return err
	}
	if stagedExists {
		if err := os.Remove(d.stagedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Remove(d.stagingDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
