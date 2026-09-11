package serve

import (
	"errors"
	"strings"
	"testing"

	core "gooru.local/gooru"
)

type fakeBackgroundOperationReservationRecovery struct {
	hiddenKinds []string
	allKinds    []string
	err         error
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	f.hiddenKinds = append(f.hiddenKinds, kind)
	if f.err != nil {
		return 0, f.err
	}
	return 1, nil
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedBackgroundOperations(kind string) (int64, error) {
	f.allKinds = append(f.allKinds, kind)
	if f.err != nil {
		return 0, f.err
	}
	return 1, nil
}

func TestRecoverBackgroundOperationReservationsTargetsDurableProducers(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{}
	if err := recoverBackgroundOperationReservations(recovery); err != nil {
		t.Fatal(err)
	}
	if len(recovery.allKinds) != 1 || recovery.allKinds[0] != backgroundUploadImportOperationKind {
		t.Fatalf("visible-capable recovery kinds = %q, want upload", recovery.allKinds)
	}
	if len(recovery.hiddenKinds) != 1 || recovery.hiddenKinds[0] != core.BackgroundTagMutationOperationKind {
		t.Fatalf("hidden recovery kinds = %q, want tag mutation", recovery.hiddenKinds)
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
