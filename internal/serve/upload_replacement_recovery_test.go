package serve

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSettleDurableActivationsRemovesReplacementRecoveryMarker(t *testing.T) {
	dir := t.TempDir()
	stagedPath := filepath.Join(dir, ".gooru-upload-staging", "operation", "photo.jpg")
	if err := os.MkdirAll(filepath.Dir(stagedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedPath, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []savedUpload{{
		name:            "photo.jpg",
		path:            stagedPath,
		destinationPath: filepath.Join(dir, "photo.jpg"),
		targetID:        "default",
		replace:         true,
	}}
	if err := prepareDurableReplacementRecoveryMarkers(files); err != nil {
		t.Fatalf("prepare replacement recovery marker: %v", err)
	}
	if _, err := os.Stat(stagedPath + durableUploadActivatedMarkerSuffix); err != nil {
		t.Fatalf("replacement recovery marker missing before settle: %v", err)
	}

	if err := settleDurableNonreplacementActivations(files); err != nil {
		t.Fatalf("settle durable activations: %v", err)
	}
	if _, err := os.Stat(stagedPath + durableUploadActivatedMarkerSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement recovery marker after settle: %v", err)
	}
}
