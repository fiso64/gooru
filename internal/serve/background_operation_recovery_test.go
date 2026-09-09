package serve

import (
	"errors"
	"strings"
	"testing"
)

type fakeBackgroundOperationReservationRecovery struct {
	kind string
	err  error
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	f.kind = kind
	if f.err != nil {
		return 0, f.err
	}
	return 1, nil
}

func TestRecoverBackgroundOperationReservationsTargetsUploads(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{}
	if err := recoverBackgroundOperationReservations(recovery); err != nil {
		t.Fatal(err)
	}
	if recovery.kind != backgroundUploadImportOperationKind {
		t.Fatalf("recovery kind = %q, want %q", recovery.kind, backgroundUploadImportOperationKind)
	}
}

func TestRecoverBackgroundOperationReservationsPropagatesFailure(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{err: errors.New("boom")}
	err := recoverBackgroundOperationReservations(recovery)
	if err == nil || !strings.Contains(err.Error(), "recover upload background operation reservations") {
		t.Fatalf("expected wrapped recovery error, got %v", err)
	}
}

func TestRecoverBackgroundOperationReservationsAllowsMissingRecovery(t *testing.T) {
	if err := recoverBackgroundOperationReservations(nil); err != nil {
		t.Fatalf("nil recovery returned %v", err)
	}
}
