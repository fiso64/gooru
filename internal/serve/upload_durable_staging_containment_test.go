package serve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStageDurableMultipartUploadRejectsStagingNamespaceSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	stagingRoot := filepath.Join(root, durableUploadStagingRootName)
	if err := os.Symlink(outside, stagingRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	server := newUploadTestServer(t, root, true, &recordingUploadLibrary{})
	req := uploadRequest(t, map[string]string{"photo.jpg": "secret"}, nil)
	if _, _, err := server.stageDurableMultipartUpload(req, testDurableUploadOperationID); err == nil {
		t.Fatal("expected staging namespace symlink escape to be rejected")
	}

	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging escape created entries outside upload target: %v", entries)
	}
}
