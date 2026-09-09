package serve

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	core "gooru.local/gooru"
)

const (
	backgroundUploadTaskKind      = "upload.import"
	backgroundUploadResourceClass = "upload"
	backgroundUploadInputVersion  = 1
	backgroundUploadPhaseStaged   = "staged"
)

type backgroundUploadCheckpoint struct {
	Phase string `json:"phase"`
}

type backgroundUploadTaskInput struct {
	Version int                        `json:"version"`
	Files   []backgroundUploadTaskFile `json:"files"`
	Tags    []string                   `json:"tags,omitempty"`
}

type backgroundUploadTaskFile struct {
	Name            string    `json:"name"`
	Path            string    `json:"path,omitempty"`
	DestinationPath string    `json:"destination_path,omitempty"`
	Size            int64     `json:"size"`
	TargetID        string    `json:"target_id"`
	Status          string    `json:"status,omitempty"`
	Error           string    `json:"error,omitempty"`
	Replace         bool      `json:"replace,omitempty"`
	SourceModTime   time.Time `json:"source_mod_time,omitempty"`
	AddedAt         time.Time `json:"added_at,omitempty"`
	ConflictPolicy  string    `json:"conflict_policy,omitempty"`
}

func backgroundUploadInitialCheckpoint() backgroundUploadCheckpoint {
	return backgroundUploadCheckpoint{Phase: backgroundUploadPhaseStaged}
}

func backgroundUploadTaskRequest(operationID string, files []savedUpload, tags []string) (core.BackgroundTaskRequest, error) {
	if operationID == "" {
		return core.BackgroundTaskRequest{}, errors.New("upload operation id is required")
	}
	input := backgroundUploadTaskInput{
		Version: backgroundUploadInputVersion,
		Files:   make([]backgroundUploadTaskFile, 0, len(files)),
		Tags:    append([]string(nil), tags...),
	}
	for _, file := range files {
		input.Files = append(input.Files, backgroundUploadTaskFile{
			Name:            file.name,
			Path:            file.path,
			DestinationPath: file.destinationPath,
			Size:            file.size,
			TargetID:        file.targetID,
			Status:          file.status,
			Error:           file.error,
			Replace:         file.replace,
			SourceModTime:   file.sourceModTime,
			AddedAt:         file.addedAt,
			ConflictPolicy:  file.conflictPolicy,
		})
	}
	if err := validateBackgroundUploadInput(input); err != nil {
		return core.BackgroundTaskRequest{}, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return core.BackgroundTaskRequest{}, fmt.Errorf("encode upload background task: %w", err)
	}
	return core.BackgroundTaskRequest{
		DedupeKey:     "import",
		Kind:          backgroundUploadTaskKind,
		SubjectKind:   "operation",
		SubjectID:     operationID,
		InputKey:      string(encoded),
		ResourceClass: backgroundUploadResourceClass,
		MaxAttempts:   5,
	}, nil
}

func decodeBackgroundUploadTask(task core.BackgroundTask) ([]savedUpload, []string, error) {
	if task.Kind != backgroundUploadTaskKind || task.SubjectKind != "operation" || task.SubjectID == "" || task.SubjectID != task.OperationID {
		return nil, nil, errors.New("upload background task has invalid operation identity")
	}
	var input backgroundUploadTaskInput
	if err := json.Unmarshal([]byte(task.InputKey), &input); err != nil {
		return nil, nil, fmt.Errorf("decode upload background task: %w", err)
	}
	if err := validateBackgroundUploadInput(input); err != nil {
		return nil, nil, err
	}
	files := make([]savedUpload, 0, len(input.Files))
	for _, file := range input.Files {
		files = append(files, savedUpload{
			name:            file.Name,
			path:            file.Path,
			destinationPath: file.DestinationPath,
			size:            file.Size,
			targetID:        file.TargetID,
			status:          file.Status,
			error:           file.Error,
			replace:         file.Replace,
			sourceModTime:   file.SourceModTime,
			addedAt:         file.AddedAt,
			conflictPolicy:  file.ConflictPolicy,
		})
	}
	return files, append([]string(nil), input.Tags...), nil
}

func validateBackgroundUploadInput(input backgroundUploadTaskInput) error {
	if input.Version != backgroundUploadInputVersion {
		return fmt.Errorf("upload background task version %d is unsupported", input.Version)
	}
	if len(input.Files) == 0 || len(input.Files) > maxUploadFiles {
		return errors.New("upload background task has invalid file count")
	}
	for index, file := range input.Files {
		if file.Name == "" || file.TargetID == "" {
			return fmt.Errorf("upload background task file %d is missing identity", index)
		}
		if file.Status != "error" && file.Status != "skipped" && file.Path == "" {
			return fmt.Errorf("upload background task file %d is missing staged path", index)
		}
		if file.Replace && file.DestinationPath == "" {
			return fmt.Errorf("upload background task file %d is missing replacement destination", index)
		}
	}
	return nil
}
