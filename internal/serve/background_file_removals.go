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
)

type backgroundFileRemovalInput struct {
	Mode         string `json:"mode"`
	PublicID     string `json:"public_id"`
	OriginalPath string `json:"original_path,omitempty"`
	StagingPath  string `json:"staging_path,omitempty"`
}

func (s *Server) backgroundFileRemovalTask(mode string, file types.FileInfo) (core.BackgroundTaskRequest, error) {
	publicID := s.publicFileID(file)
	input := backgroundFileRemovalInput{Mode: mode, PublicID: publicID}
	if mode == "delete" {
		managedPath, ok := s.managedDeleteCandidatePath(fileStoragePath(file))
		if !ok {
			return core.BackgroundTaskRequest{}, ErrFileNotManaged
		}
		token, err := newFileRemovalToken()
		if err != nil {
			return core.BackgroundTaskRequest{}, err
		}
		stagingDir := filepath.Join(filepath.Dir(managedPath), ".gooru-delete-"+token)
		input.OriginalPath = managedPath
		input.StagingPath = filepath.Join(stagingDir, filepath.Base(managedPath))
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return core.BackgroundTaskRequest{}, fmt.Errorf("encode file removal task: %w", err)
	}
	return core.BackgroundTaskRequest{
		DedupeKey:     "file:" + publicID,
		Kind:          backgroundFileRemovalTaskKind,
		SubjectKind:   "file",
		SubjectID:     publicID,
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

func (s *Server) resumeManagedFileDeletion(ctx context.Context, input backgroundFileRemovalInput) error {
	staged, err := persistedStagedFileDeletion(input.OriginalPath, input.StagingPath)
	if err != nil {
		return err
	}
	stagedExists, err := pathExists(staged.stagedPath)
	if err != nil {
		return err
	}
	if !stagedExists {
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
	if err := os.Mkdir(d.stagingDir, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("prepare file deletion: %w", err)
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
	exists, err := pathExists(d.stagedPath)
	if err != nil || !exists {
		return err
	}
	if err := os.Rename(d.stagedPath, d.originalPath); err != nil {
		return err
	}
	_ = os.Remove(d.stagingDir)
	return nil
}

func (d *stagedFileDeletion) commitMissingOK() error {
	if err := os.Remove(d.stagedPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
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
