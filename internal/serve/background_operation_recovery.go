package serve

import (
	"fmt"

	core "gooru.local/gooru"
)

const backgroundUploadImportOperationKind = "upload_import"

type backgroundOperationReservationRecovery interface {
	CancelUnattachedHiddenBackgroundOperations(string) (int64, error)
}

// recoverBackgroundOperationReservations releases producer admission reservations
// left behind before a durable task was attached. Startup invokes this before
// constructing workers and before the HTTP server begins accepting new producer
// requests, so only pre-existing hidden reservations are eligible.
func recoverBackgroundOperationReservations(recovery backgroundOperationReservationRecovery) error {
	if recovery == nil {
		return nil
	}
	for _, item := range []struct {
		kind string
		name string
	}{
		{kind: backgroundUploadImportOperationKind, name: "upload"},
		{kind: core.BackgroundTagMutationOperationKind, name: "tag mutation"},
	} {
		if _, err := recovery.CancelUnattachedHiddenBackgroundOperations(item.kind); err != nil {
			return fmt.Errorf("recover %s background operation reservations: %w", item.name, err)
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
