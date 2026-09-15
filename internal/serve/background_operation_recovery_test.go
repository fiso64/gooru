package serve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "gooru.local/gooru"
)

type fakeBackgroundOperationReservationRecovery struct {
	hiddenKinds  []string
	listedKinds  []string
	allKinds     []string
	operationIDs []string
	err          error
	cancelErr    error
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedHiddenBackgroundOperations(kind string) (int64, error) {
	f.hiddenKinds = append(f.hiddenKinds, kind)
	if f.err != nil {
		return 0, f.err
	}
	return 1, nil
}

func (f *fakeBackgroundOperationReservationRecovery) ListUnattachedBackgroundOperationIDs(kind string) ([]string, error) {
	f.listedKinds = append(f.listedKinds, kind)
	if f.err != nil {
		return nil, f.err
	}
	return append([]string(nil), f.operationIDs...), nil
}

func (f *fakeBackgroundOperationReservationRecovery) CancelUnattachedBackgroundOperationIDs(kind string) ([]string, error) {
	f.allKinds = append(f.allKinds, kind)
	if f.cancelErr != nil {
		return nil, f.cancelErr
	}
	return append([]string(nil), f.operationIDs...), nil
}

func TestRecoverBackgroundOperationReservationsTargetsDurableProducers(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{}
	if err := recoverBackgroundOperationReservations(recovery, nil); err != nil {
		t.Fatal(err)
	}
	if len(recovery.listedKinds) != 1 || recovery.listedKinds[0] != backgroundUploadImportOperationKind {
		t.Fatalf("listed recovery kinds = %q, want upload", recovery.listedKinds)
	}
	if len(recovery.allKinds) != 1 || recovery.allKinds[0] != backgroundUploadImportOperationKind {
		t.Fatalf("visible-capable recovery kinds = %q, want upload", recovery.allKinds)
	}
	if len(recovery.hiddenKinds) != 1 || recovery.hiddenKinds[0] != core.BackgroundTagMutationOperationKind {
		t.Fatalf("hidden recovery kinds = %q, want tag mutation", recovery.hiddenKinds)
	}
}

func TestRecoverBackgroundOperationReservationsReclaimsOnlyCanceledUploadStaging(t *testing.T) {
	root := t.TempDir()
	staleID := "operation-0123456789abcdef0123456789abcdef"
	activeID := "operation-fedcba9876543210fedcba9876543210"
	staleDir, err := durableUploadStagingDir(root, staleID)
	if err != nil {
		t.Fatal(err)
	}
	activeDir, err := durableUploadStagingDir(root, activeID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(staleDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(activeDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staleDir, "partial"), []byte("stale"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(activeDir, "partial"), []byte("active"), 0600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(root, ".gooru-upload-unrelated")
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}

	recovery := &fakeBackgroundOperationReservationRecovery{operationIDs: []string{staleID}}
	if err := recoverBackgroundOperationReservations(recovery, []UploadTarget{{Path: root}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staleDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staging still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(activeDir, "partial")); err != nil {
		t.Fatalf("active staging was removed: %v", err)
	}
	if got, err := os.ReadFile(unrelated); err != nil || string(got) != "keep" {
		t.Fatalf("unrelated upload file changed: %q, %v", got, err)
	}
}

func TestRecoverBackgroundOperationReservationsRejectsUnsafeOperationIDBeforeCancel(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{operationIDs: []string{"../escape"}}
	err := recoverBackgroundOperationReservations(recovery, []UploadTarget{{Path: t.TempDir()}})
	if err == nil || !strings.Contains(err.Error(), "invalid recovered upload operation id") {
		t.Fatalf("expected unsafe-id failure, got %v", err)
	}
	if len(recovery.allKinds) != 0 {
		t.Fatalf("canceled recovery kinds after reclaim failure = %q, want none", recovery.allKinds)
	}
}

func TestRecoverBackgroundOperationReservationsPropagatesFailure(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{err: errors.New("boom")}
	err := recoverBackgroundOperationReservations(recovery, nil)
	if err == nil || !strings.Contains(err.Error(), "recover upload background operation reservations") {
		t.Fatalf("expected wrapped recovery error, got %v", err)
	}
}

func TestRecoverBackgroundOperationReservationsPropagatesCancellationFailure(t *testing.T) {
	recovery := &fakeBackgroundOperationReservationRecovery{cancelErr: errors.New("boom")}
	err := recoverBackgroundOperationReservations(recovery, nil)
	if err == nil || !strings.Contains(err.Error(), "cancel recovered upload background operation reservations") {
		t.Fatalf("expected wrapped cancellation error, got %v", err)
	}
	if len(recovery.hiddenKinds) != 0 {
		t.Fatalf("tag recovery ran after upload cancellation failure: %q", recovery.hiddenKinds)
	}
}

func TestRecoverBackgroundOperationReservationsAllowsMissingRecovery(t *testing.T) {
	if err := recoverBackgroundOperationReservations(nil, nil); err != nil {
		t.Fatalf("nil recovery returned %v", err)
	}
}
