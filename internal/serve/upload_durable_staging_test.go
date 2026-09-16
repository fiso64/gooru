package serve

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const testDurableUploadOperationID = "operation-0123456789abcdef0123456789abcdef"

func TestStageDurableMultipartUploadDoesNotPublishDestinationBeforeAttachment(t *testing.T) {
	root := t.TempDir()
	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	req := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)

	_, saved, err := server.stageDurableMultipartUpload(req, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved files = %d, want 1", len(saved))
	}
	destination := filepath.Join(root, "photo.jpg")
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("normal destination exists before durable attachment: %v", err)
	}
	wantDir, err := durableUploadStagingDir(root, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	requestDir := filepath.Dir(saved[0].path)
	if filepath.Clean(filepath.Dir(requestDir)) != filepath.Clean(wantDir) {
		t.Fatalf("staged path = %q, want request staging directory below %q", saved[0].path, wantDir)
	}
	if saved[0].destinationPath != destination {
		t.Fatalf("destination path = %q, want %q", saved[0].destinationPath, destination)
	}
	if got := string(mustReadFile(t, saved[0].path)); got != "hello" {
		t.Fatalf("staged content = %q, want hello", got)
	}
}

func TestStageDurableMultipartUploadFailureDoesNotRemoveSiblingRequestStaging(t *testing.T) {
	root := t.TempDir()
	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	firstReq := uploadRequest(t, map[string]string{"first.jpg": "first"}, nil)
	_, firstSaved, err := server.stageDurableMultipartUpload(firstReq, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstSaved) != 1 {
		t.Fatalf("first saved files = %d, want 1", len(firstSaved))
	}

	failedReq := uploadRequest(t, map[string]string{"second.jpg": "second"}, nil)
	ctx, cancel := context.WithCancel(failedReq.Context())
	cancel()
	failedReq = failedReq.WithContext(ctx)
	if _, _, err := server.stageDurableMultipartUpload(failedReq, testDurableUploadOperationID); !errors.Is(err, errUploadReceivingCanceled) {
		t.Fatalf("failed sibling error = %v, want %v", err, errUploadReceivingCanceled)
	}
	if got := string(mustReadFile(t, firstSaved[0].path)); got != "first" {
		t.Fatalf("surviving staged content = %q, want first", got)
	}
}

func TestActivateDurableUploadDestinationIsReplaySafe(t *testing.T) {
	root := t.TempDir()
	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	req := uploadRequest(t, map[string]string{"photo.jpg": "hello"}, nil)
	_, saved, err := server.stageDurableMultipartUpload(req, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := activateSavedDurableUploads(saved); err != nil {
		t.Fatal(err)
	}
	if err := activateSavedDurableUploads(saved); err != nil {
		t.Fatalf("replay activation: %v", err)
	}
	if got := string(mustReadFile(t, saved[0].destinationPath)); got != "hello" {
		t.Fatalf("activated content = %q, want hello", got)
	}
	if _, err := os.Stat(saved[0].path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged file still exists after activation: %v", err)
	}
	if _, err := os.Stat(saved[0].path + durableUploadActivatedMarkerSuffix); err != nil {
		t.Fatalf("activation marker missing: %v", err)
	}
	if err := settleDurableNonreplacementActivations(saved); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(saved[0].path + durableUploadActivatedMarkerSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("activation marker still exists after settlement: %v", err)
	}
}

func TestActivateDurableUploadDestinationRenamesDestinationRace(t *testing.T) {
	root := t.TempDir()
	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	req := uploadRequest(t, map[string]string{"photo.jpg": "staged"}, nil)
	_, saved, err := server.stageDurableMultipartUpload(req, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	originalDestination := saved[0].destinationPath
	if err := os.WriteFile(originalDestination, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := activateSavedDurableUploads(saved); err != nil {
		t.Fatalf("activation error = %v, want rename retry", err)
	}
	if got := string(mustReadFile(t, originalDestination)); got != "racer" {
		t.Fatalf("racing destination content = %q, want racer", got)
	}
	if got, want := filepath.Base(saved[0].destinationPath), "photo-1.jpg"; got != want {
		t.Fatalf("renamed destination = %q, want %q", got, want)
	}
	if got := string(mustReadFile(t, saved[0].destinationPath)); got != "staged" {
		t.Fatalf("renamed upload content = %q, want staged", got)
	}
	staged := durableStagedUploads(saved)
	if len(staged) != 1 || staged[0].Name != "photo-1.jpg" {
		t.Fatalf("durable staged upload name = %#v, want photo-1.jpg", staged)
	}
}

func TestActivateDurableUploadDestinationContinuesOriginalSuffixSequence(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	req := uploadRequest(t, map[string]string{"photo.jpg": "staged"}, nil)
	_, saved, err := server.stageDurableMultipartUpload(req, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(saved[0].destinationPath), "photo-1.jpg"; got != want {
		t.Fatalf("preflight destination = %q, want %q", got, want)
	}
	if saved[0].name != "photo.jpg" {
		t.Fatalf("stable requested name = %q, want photo.jpg", saved[0].name)
	}
	if err := os.WriteFile(saved[0].destinationPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := activateSavedDurableUploads(saved); err != nil {
		t.Fatal(err)
	}
	if got, want := filepath.Base(saved[0].destinationPath), "photo-2.jpg"; got != want {
		t.Fatalf("retry destination = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, "photo-1-1.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retry nested the selected suffix instead of continuing the original sequence: %v", err)
	}
}

func TestActivateDurableUploadDestinationRecoversRetriedDestinationFromMarker(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"photo.jpg", "photo-1.jpg"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("racer"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	requestDir := filepath.Join(root, durableUploadStagingRootName, testDurableUploadOperationID, "request-crash")
	if err := os.MkdirAll(requestDir, 0700); err != nil {
		t.Fatal(err)
	}
	stagedPath := filepath.Join(requestDir, "staged")
	if err := os.WriteFile(stagedPath, []byte("staged"), 0600); err != nil {
		t.Fatal(err)
	}
	markerPath := stagedPath + durableUploadActivatedMarkerSuffix
	if err := os.Link(stagedPath, markerPath); err != nil {
		t.Fatal(err)
	}
	ownedDestination := filepath.Join(root, "photo-2.jpg")
	if err := os.Link(markerPath, ownedDestination); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stagedPath); err != nil {
		t.Fatal(err)
	}

	files := []savedUpload{{name: "photo.jpg", path: stagedPath, destinationPath: filepath.Join(root, "photo-1.jpg"), conflictPolicy: "rename"}}
	if err := activateSavedDurableUploads(files); err != nil {
		t.Fatalf("recover activation: %v", err)
	}
	if files[0].destinationPath != ownedDestination {
		t.Fatalf("recovered destination = %q, want %q", files[0].destinationPath, ownedDestination)
	}
	if _, err := os.Stat(filepath.Join(root, "photo-3.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replay allocated an extra destination instead of recovering ownership: %v", err)
	}
}

func TestActivateDurableUploadDestinationErrorPolicyRejectsDestinationRace(t *testing.T) {
	root := t.TempDir()
	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	req := uploadRequestWithConflict(t, map[string]string{"photo.jpg": "staged"}, nil, "", "error")
	_, saved, err := server.stageDurableMultipartUpload(req, testDurableUploadOperationID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saved[0].destinationPath, []byte("racer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := activateSavedDurableUploads(saved); !errors.Is(err, errUploadConflict) {
		t.Fatalf("activation error = %v, want upload conflict", err)
	}
	if got := string(mustReadFile(t, saved[0].destinationPath)); got != "racer" {
		t.Fatalf("racing destination content = %q, want racer", got)
	}
}

func TestChooseDurableUploadDestinationReservesEarlierBatchNames(t *testing.T) {
	root := t.TempDir()
	reserved := map[string]struct{}{}
	first, err := chooseDurableUploadDestination(root, "photo.jpg", "rename", reserved)
	if err != nil {
		t.Fatalf("first destination = %q err=%v", first, err)
	}
	reserved[first] = struct{}{}
	second, err := chooseDurableUploadDestination(root, "photo.jpg", "rename", reserved)
	if err != nil {
		t.Fatalf("second destination = %q err=%v", second, err)
	}
	if got, want := filepath.Base(second), "photo-1.jpg"; got != want {
		t.Fatalf("second destination = %q, want %q", got, want)
	}
}

func TestDurableStagedUploadsKeepsLegacyPublishedTaskCompatible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.jpg")
	files := []savedUpload{{name: "legacy.jpg", path: path, destinationPath: path, targetID: "default"}}
	staged := durableStagedUploads(files)
	if len(staged) != 1 || staged[0].Path != path {
		t.Fatalf("legacy staged uploads = %#v, want path %q", staged, path)
	}
	if err := activateSavedDurableUploads(files); err != nil {
		t.Fatalf("legacy activation should be a no-op: %v", err)
	}
}

func TestChooseDurableUploadDestinationErrorRejectsExistingOrReservedName(t *testing.T) {
	root := t.TempDir()
	existing := filepath.Join(root, "photo.jpg")
	if err := os.WriteFile(existing, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := chooseDurableUploadDestination(root, "photo.jpg", "error", map[string]struct{}{}); !errors.Is(err, errUploadConflict) {
		t.Fatalf("existing destination error = %v, want upload conflict", err)
	}
	if err := os.Remove(existing); err != nil {
		t.Fatal(err)
	}
	reserved := map[string]struct{}{existing: {}}
	if _, err := chooseDurableUploadDestination(root, "photo.jpg", "error", reserved); !errors.Is(err, errUploadConflict) {
		t.Fatalf("reserved destination error = %v, want upload conflict", err)
	}
}
