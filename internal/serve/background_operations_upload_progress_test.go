package serve

import (
	"testing"
	"time"

	core "gooru.local/gooru"
)

func TestBackgroundOperationDTOProjectsAggregateAndRowUploadProgress(t *testing.T) {
	createdAt := time.Now().UTC()
	operation := core.BackgroundOperationState{
		ID: "operation-upload-progress", Kind: backgroundUploadImportOperationKind, Visible: true,
		Status: core.BackgroundWorkRunning, ProgressTotal: 1, CreatedAt: createdAt,
	}
	reader := &fakeBackgroundOperationReader{
		checkpoints: map[string]backgroundUploadCheckpoint{
			operation.ID: {
				Phase:                backgroundUploadPhaseActivated,
				FileTotal:            1000,
				FilesCompleted:       420,
				FilesCompletedPrefix: 417,
			},
		},
	}
	server := &Server{backgroundOperations: reader}

	dto := server.backgroundOperationDTO(operation)
	if dto.ProgressTotal != 1000 || dto.ProgressCompleted != 420 || dto.ProgressCompletedPrefix != 417 {
		t.Fatalf("projected upload progress = %+v", dto)
	}
}

func TestBackgroundOperationDTOClampsUploadRowPrefixToAggregateCompletion(t *testing.T) {
	operation := core.BackgroundOperationState{
		ID: "operation-upload-progress", Kind: backgroundUploadImportOperationKind, Visible: true,
		Status: core.BackgroundWorkRunning, CreatedAt: time.Now().UTC(),
	}
	reader := &fakeBackgroundOperationReader{
		checkpoints: map[string]backgroundUploadCheckpoint{
			operation.ID: {
				Phase:                backgroundUploadPhaseActivated,
				FileTotal:            10,
				FilesCompleted:       4,
				FilesCompletedPrefix: 9,
			},
		},
	}
	server := &Server{backgroundOperations: reader}

	dto := server.backgroundOperationDTO(operation)
	if dto.ProgressCompleted != 4 || dto.ProgressCompletedPrefix != 4 {
		t.Fatalf("clamped upload progress = %+v", dto)
	}
}
