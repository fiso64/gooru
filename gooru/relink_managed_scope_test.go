package gooru

import (
	"path/filepath"
	"testing"
)

func TestRelinkIgnoresManagedBackingPathOutsideLogicalScope(t *testing.T) {
	client := newManagedRelinkClient(t)
	scanDir := t.TempDir()
	physicalPath := filepath.Join(scanDir, "managed.bin")
	logicalPath := filepath.Join(t.TempDir(), "library", "upload.bin")
	addManagedTestLocation(t, client, logicalPath, physicalPath, []byte("managed contents"))

	for _, verifyHash := range []bool{false, true} {
		needsRelink, err := client.NeedsRelink([]string{scanDir}, verifyHash)
		if err != nil {
			t.Fatal(err)
		}
		if needsRelink {
			t.Fatalf("NeedsRelink(alwaysVerifyHash=%v) = true for managed backing alias outside logical scope", verifyHash)
		}
	}

	result, err := client.Relink([]string{scanDir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ProposedMoves) != 0 || len(result.ProposedAdds) != 0 || len(result.ProposedDeletes) != 0 {
		t.Fatalf("managed backing alias produced relink changes: %+v", result)
	}
}
