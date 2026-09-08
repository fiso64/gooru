package serve

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const opaqueManagedNameBytes = 24

// OpaqueManagedStoragePath chooses a fresh extensionless physical name beside
// the canonical logical path. The hash/key parameters are retained for the
// recovery-branch call contract but deliberately do not influence the name:
// protected physical names must not encode content or public identity.
func OpaqueManagedStoragePath(logicalPath string, _ string, _ []byte) (string, error) {
	dir := filepath.Dir(logicalPath)
	for i := 0; i < 32; i++ {
		buf := make([]byte, opaqueManagedNameBytes)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate opaque managed storage name: %w", err)
		}
		candidate := filepath.Join(dir, hex.EncodeToString(buf))
		if _, err := os.Lstat(candidate); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("inspect opaque managed storage candidate: %w", err)
		}
	}
	return "", fmt.Errorf("could not choose an unused opaque managed storage name")
}

func moveManagedFileToOpaqueStorage(logicalPath, contentHash string, key []byte) (string, error) {
	physicalPath, err := OpaqueManagedStoragePath(logicalPath, contentHash, key)
	if err != nil {
		return "", err
	}
	if err := os.Rename(logicalPath, physicalPath); err != nil {
		return "", fmt.Errorf("move managed upload to opaque storage: %w", err)
	}
	return physicalPath, nil
}
