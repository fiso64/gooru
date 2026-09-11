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
	if dto.Progress == nil || *dto.Progress < 0.709999 || *dto.Progress > 0.710001 {
		t.Fatalf("overall upload progress = %v, want 0.71", dto.Progress)
	}
}

func TestBackgroundOperationDTOProjectsReceivingUploadProgress(t *testing.T) {
	operation := core.BackgroundOperationState{ID: "operation-upload-receiving", Kind: backgroundUploadImportOperationKind, Visible: true, Status: core.BackgroundWorkPending, ProgressTotal: 1, CreatedAt: time.Now().UTC()}
	reader := &fakeBackgroundOperationReader{checkpoints: map[string]backgroundUploadCheckpoint{
		operation.ID: backgroundUploadReceivingCheckpoint(1000, 400),
	}}
	dto := (&Server{backgroundOperations: reader}).backgroundOperationDTO(operation)
	if dto.Progress == nil || *dto.Progress < 0.199999 || *dto.Progress > 0.200001 {
		t.Fatalf("receiving progress = %v, want 0.2", dto.Progress)
	}
	if dto.ProgressTotal != 1 || dto.ProgressCompleted != 0 {
		t.Fatalf("receiving should preserve raw durable counters: %+v", dto)
	}
}

func TestBackgroundOperationDTOLeavesUnknownLengthReceivingIndeterminate(t *testing.T) {
	operation := core.BackgroundOperationState{ID: "operation-upload-receiving", Kind: backgroundUploadImportOperationKind, Visible: true, Status: core.BackgroundWorkPending, ProgressTotal: 1, CreatedAt: time.Now().UTC()}
	reader := &fakeBackgroundOperationReader{checkpoints: map[string]backgroundUploadCheckpoint{
		operation.ID: backgroundUploadReceivingCheckpoint(0, 400),
	}}
	dto := (&Server{backgroundOperations: reader}).backgroundOperationDTO(operation)
	if dto.Progress != nil {
		t.Fatalf("unknown-length receiving progress = %v, want nil", *dto.Progress)
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
