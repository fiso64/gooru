//go:build linux

package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagedUploadProvisioningRejectsMissingState(t *testing.T) {
	// No test may create a managed state directory under /var/lib.
	path := "/etc/gooru/owntest-zz916/serve.yaml"
	state, managed := managedStateDirectoryFromConfig(path)
	if !managed {
		t.Fatal("expected managed configuration")
	}
	if _, err := os.Lstat(state); !os.IsNotExist(err) {
		t.Skipf("unexpected real managed state path %q: %v", state, err)
	}
	err := verifyManagedUploadStateOwner(path)
	if err == nil || !strings.Contains(err.Error(), "must already exist") {
		t.Fatalf("missing managed state must fail before creation: %v", err)
	}
	if _, err := os.Lstat(state); !os.IsNotExist(err) {
		t.Fatalf("managed state created unexpectedly: %v", err)
	}
}

func TestExistingDefaultUploadTargetMustBelongToCurrentIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "uploads")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := verifyDefaultUploadDirectoryOwner(path); err != nil {
		t.Fatalf("current identity must own its own upload directory: %v", err)
	}
}
