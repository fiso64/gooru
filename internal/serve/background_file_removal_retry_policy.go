package serve

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	core "gooru.local/gooru"
)

func backgroundFileRemovalApplyDeleteRetryPolicy(task core.BackgroundTaskRequest) (core.BackgroundTaskRequest, error) {
	if task.Kind != backgroundFileRemovalTaskKind || task.SubjectKind != "file_batch" || task.SubjectID != "selection" || task.InputKey == "" {
		return core.BackgroundTaskRequest{}, fmt.Errorf("file deletion task has invalid identity")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return core.BackgroundTaskRequest{}, fmt.Errorf("generate file deletion cleanup identity: %w", err)
	}
	// Physical deletion is never replayed after an attempt ends, including lease
	// expiry. Terminal compensation reconciles the persisted staging state instead:
	// restore it while the DB location still exists, or purge it after commit.
	task.MaxAttempts = 1
	task.TerminalFailureCleanup = &core.BackgroundTaskCleanupRequest{
		DedupeKey:     "file-removal-terminal-cleanup:" + hex.EncodeToString(nonce[:]),
		Kind:          backgroundFileRemovalCleanupTaskKind,
		SubjectKind:   task.SubjectKind,
		SubjectID:     task.SubjectID,
		InputKey:      task.InputKey,
		ResourceClass: backgroundFileRemovalResourceClass,
		Priority:      backgroundFileRemovalCleanupPriority,
		MaxAttempts:   5,
	}
	return task, nil
}
