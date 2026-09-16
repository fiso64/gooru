package serve

import (
	"fmt"
	"os"
	"path/filepath"

	core "gooru.local/gooru"
)

const backgroundUploadImportOperationKind = "upload_import"

type backgroundOperationReservationRecovery interface {
	CancelUnattachedHiddenBackgroundOperations(string) (int64, error)
	ListUnattachedBackgroundOperationIDs(string) ([]string, error)
	CancelUnattachedBackgroundOperationIDs(string) ([]string, error)
}

// recoverBackgroundOperationReservations releases producer admission reservations
// left behind before a durable task was attached. Startup invokes this before
// constructing workers and before the HTTP server begins accepting new producer
// requests. Upload staging cleanup is restricted to exact operation ids selected
// by durable recovery, and cleanup completes before cancellation so a filesystem
// failure remains discoverable and retryable on the next startup.
func recoverBackgroundOperationReservations(recovery backgroundOperationReservationRecovery, uploadTargets []UploadTarget) error {
	if recovery == nil {
		return nil
	}
	operationIDs, err := recovery.ListUnattachedBackgroundOperationIDs(backgroundUploadImportOperationKind)
	if err != nil {
		return fmt.Errorf("recover upload background operation reservations: %w", err)
	}
	if err := reclaimInterruptedUploadStaging(uploadTargets, operationIDs); err != nil {
		return fmt.Errorf("reclaim interrupted upload staging: %w", err)
	}
	if _, err := recovery.CancelUnattachedBackgroundOperationIDs(backgroundUploadImportOperationKind); err != nil {
		return fmt.Errorf("cancel recovered upload background operation reservations: %w", err)
	}
	if _, err := recovery.CancelUnattachedHiddenBackgroundOperations(core.BackgroundTagMutationOperationKind); err != nil {
		return fmt.Errorf("recover tag mutation background operation reservations: %w", err)
	}
	return nil
}

func reclaimInterruptedUploadStaging(targets []UploadTarget, operationIDs []string) error {
	roots := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		root := filepath.Clean(target.Path)
		if root != "." && root != "" {
			roots[root] = struct{}{}
		}
	}
	for _, operationID := range operationIDs {
		if !validDurableUploadOperationID(operationID) {
			return fmt.Errorf("invalid recovered upload operation id %q", operationID)
		}
		for root := range roots {
			dir, err := durableUploadStagingDir(root, operationID)
			if err != nil {
				return err
			}
			if err := os.RemoveAll(dir); err != nil {
				return fmt.Errorf("remove upload staging for %s: %w", operationID, err)
			}
			_ = os.Remove(filepath.Join(root, durableUploadStagingRootName))
		}
	}
	return nil
}

// CancelUnattachedHiddenBackgroundOperations exposes the core startup recovery
// primitive through the server library facade for producer composition.
func (l *GooruLibrary) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	return l.client.CancelUnattachedHiddenBackgroundOperations(kind)
}

var _ backgroundOperationReservationRecovery = (*core.Client)(nil)
