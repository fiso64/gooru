package serve

import (
	"errors"
	"strings"
	"testing"

	core "gooru.local/gooru"
)

type fakeBackgroundOperationReservationRecovery struct {
	kinds []string
	err   error
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	f.kinds = append(f.kinds, kind)
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
	want := []string{backgroundUploadImportOperationKind, core.BackgroundTagMutationOperationKind}
	if len(recovery.kinds) != len(want) || recovery.kinds[0] != want[0] || recovery.kinds[1] != want[1] {
		t.Fatalf("recovery kinds = %q, want %q", recovery.kinds, want)
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
