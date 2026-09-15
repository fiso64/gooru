package serve

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
)

const (
	backgroundUploadTaskKind       = "upload.import"
	backgroundUploadResourceClass  = "upload"
	backgroundUploadInputVersion   = 1
	backgroundUploadPhaseReceiving = "receiving"
	backgroundUploadPhaseStaged    = "staged"
	backgroundUploadPhaseActivated = "activated"
	backgroundUploadPhaseImported  = "imported"
)

type backgroundUploadCheckpoint struct {
	Phase                  string                                  `json:"phase"`
	Replacements           []backgroundUploadReplacementCheckpoint `json:"replacements,omitempty"`
	Response               *UploadImportResponse                   `json:"response,omitempty"`
	FileTotal              int                                     `json:"file_total,omitempty"`
	FilesCompleted         int                                     `json:"files_completed,omitempty"`
	FilesCompletedPrefix   int                                     `json:"files_completed_prefix,omitempty"`
	TransportBytesTotal    int64                                   `json:"transport_bytes_total,omitempty"`
	TransportBytesReceived int64                                   `json:"transport_bytes_received,omitempty"`
}

type backgroundUploadReplacementCheckpoint struct {
	Index       int  `json:"index"`
	HadOriginal bool `json:"had_original"`
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
	Tags            *[]string `json:"tags,omitempty"`
}

func backgroundUploadReceivingCheckpoint(total, received int64) backgroundUploadCheckpoint {
	if total < 0 {
		total = 0
	}
	if received < 0 {
		received = 0
	}
	if total > 0 && received > total {
		received = total
	}
	return backgroundUploadCheckpoint{
		Phase:                  backgroundUploadPhaseReceiving,
		TransportBytesTotal:    total,
		TransportBytesReceived: received,
	}
}

func backgroundUploadInitialCheckpoint(fileTotal ...int) backgroundUploadCheckpoint {
	total := 0
	if len(fileTotal) > 0 {
		total = fileTotal[0]
	}
	return backgroundUploadCheckpoint{Phase: backgroundUploadPhaseStaged, FileTotal: total}
}

func backgroundUploadActivatedCheckpoint(activated []activatedSavedReplacement, progress ...int) backgroundUploadCheckpoint {
	checkpoint := backgroundUploadCheckpoint{Phase: backgroundUploadPhaseActivated}
	checkpoint.Replacements = backgroundUploadReplacementCheckpoints(activated)
	if len(progress) > 0 {
		checkpoint.FileTotal = progress[0]
	}
	if len(progress) > 1 {
		checkpoint.FilesCompleted = progress[1]
	}
	if len(progress) > 2 {
		checkpoint.FilesCompletedPrefix = progress[2]
	}
	return checkpoint
}

func backgroundUploadImportedCheckpoint(activated []activatedSavedReplacement, response UploadImportResponse) backgroundUploadCheckpoint {
	total := len(response.Files)
	checkpoint := backgroundUploadCheckpoint{
		Phase:                backgroundUploadPhaseImported,
		Replacements:         backgroundUploadReplacementCheckpoints(activated),
		Response:             &response,
		FileTotal:            total,
		FilesCompleted:       total,
		FilesCompletedPrefix: total,
	}
	return checkpoint
}

func shouldPersistUploadProgress(completed, total int) bool {
	if completed <= 0 || total <= 0 {
		return false
	}
	step := total / 50
	if step < 1 {
		step = 1
	}
	return completed == total || completed%step == 0
}

func backgroundUploadReplacementCheckpoints(activated []activatedSavedReplacement) []backgroundUploadReplacementCheckpoint {
	if len(activated) == 0 {
		return nil
	}
	result := make([]backgroundUploadReplacementCheckpoint, 0, len(activated))
	for _, item := range activated {
		result = append(result, backgroundUploadReplacementCheckpoint{Index: item.index, HadOriginal: item.replacement.hadOriginal})
	}
	return result
}

func activatedSavedReplacementsFromCheckpoint(files []savedUpload, checkpoint backgroundUploadCheckpoint) ([]activatedSavedReplacement, error) {
	if checkpoint.Phase != backgroundUploadPhaseActivated && checkpoint.Phase != backgroundUploadPhaseImported {
		return nil, fmt.Errorf("upload checkpoint phase %q has no activated replacements", checkpoint.Phase)
	}
	replaceCount := 0
	for _, file := range files {
		if file.replace {
			replaceCount++
		}
	}
	if len(checkpoint.Replacements) != replaceCount {
		return nil, errors.New("upload checkpoint replacement count does not match task input")
	}
	savedByIndex := make(map[int]backgroundUploadReplacementCheckpoint, len(checkpoint.Replacements))
	for _, saved := range checkpoint.Replacements {
		if saved.Index < 0 || saved.Index >= len(files) {
			return nil, errors.New("upload checkpoint replacement index is out of range")
		}
		if _, ok := savedByIndex[saved.Index]; ok {
			return nil, errors.New("upload checkpoint replacement index is duplicated")
		}
		file := files[saved.Index]
		if !file.replace || file.path == "" || file.destinationPath == "" {
			return nil, errors.New("upload checkpoint replacement does not match task input")
		}
		savedByIndex[saved.Index] = saved
	}
	activated := make([]activatedSavedReplacement, 0, len(checkpoint.Replacements))
	for index, file := range files {
		if !file.replace {
			continue
		}
		saved, ok := savedByIndex[index]
		if !ok {
			return nil, errors.New("upload checkpoint replacement does not match task input")
		}
		activated = append(activated, activatedSavedReplacement{
			index: index,
			replacement: activatedReplacement{
				finalPath:            file.destinationPath,
				backupPath:           file.path + ".backup",
				noOriginalMarkerPath: file.path + ".no-original",
				hadOriginal:          saved.HadOriginal,
			},
		})
	}
	return activated, nil
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
			Tags:            cloneUploadTags(file.tags),
		})
	}
	if err := validateBackgroundUploadInput(input); err != nil {
		return core.BackgroundTaskRequest{}, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return core.BackgroundTaskRequest{}, fmt.Errorf("encode upload background task: %w", err)
	}
	terminalCleanup, err := backgroundUploadTerminalFailureCleanup(operationID)
	if err != nil {
		return core.BackgroundTaskRequest{}, err
	}
	return core.BackgroundTaskRequest{
		DedupeKey:              "import",
		Kind:                   backgroundUploadTaskKind,
		SubjectKind:            "operation",
		SubjectID:              operationID,
		InputKey:               string(encoded),
		ResourceClass:          backgroundUploadResourceClass,
		MaxAttempts:            5,
		TerminalFailureCleanup: terminalCleanup,
	}, nil
}

func backgroundUploadTerminalFailureCleanup(operationID string) (*core.BackgroundTaskCleanupRequest, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generate upload terminal cleanup identity: %w", err)
	}
	cleanup := backgroundUploadCleanupTaskRequest(operationID)
	return &core.BackgroundTaskCleanupRequest{
		DedupeKey:     cleanup.DedupeKey + ":task:" + hex.EncodeToString(nonce[:]),
		Kind:          cleanup.Kind,
		SubjectKind:   cleanup.SubjectKind,
		SubjectID:     cleanup.SubjectID,
		InputKey:      cleanup.InputKey,
		ResourceClass: cleanup.ResourceClass,
		Priority:      cleanup.Priority,
		MaxAttempts:   cleanup.MaxAttempts,
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
			tags:            cloneUploadTags(file.Tags),
		})
	}
	return files, append([]string(nil), input.Tags...), nil
}

func validateBackgroundUploadInput(input backgroundUploadTaskInput) error {
	if input.Version != backgroundUploadInputVersion {
		return fmt.Errorf("upload background task version %d is unsupported", input.Version)
	}
	if len(input.Files) == 0 {
		return errors.New("upload background task has invalid file count")
	}
	if err := query.ValidateTags(input.Tags); err != nil {
		return fmt.Errorf("upload background task has invalid global tags: %w", err)
	}
	for index, file := range input.Files {
		if file.TargetID == "" || (file.Name == "" && file.Status != "error") {
			return fmt.Errorf("upload background task file %d is missing identity", index)
		}
		if file.Status != "error" && file.Status != "skipped" && file.Path == "" {
			return fmt.Errorf("upload background task file %d is missing staged path", index)
		}
		if file.Replace && file.DestinationPath == "" {
			return fmt.Errorf("upload background task file %d is missing replacement destination", index)
		}
		if file.Tags != nil {
			if err := query.ValidateTags(*file.Tags); err != nil {
				return fmt.Errorf("upload background task file %d has invalid tags: %w", index, err)
			}
		}
	}
	return nil
}
