//go:build linux || darwin

package serve

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

func verifyDefaultUploadDirectoryOwner(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect default upload directory %q: %w", path, err)
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || metadata.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("default upload directory %q must belong to the current service user", path)
	}
	return nil
}

// Managed service state must already have been provisioned by the service
// manager for this exact identity. Administrative CLI commands must not
// provision or chmod another service account's private directory.
func verifyManagedUploadStateOwner(configPath string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	state, managed := managedStateDirectoryFromConfig(configPath)
	if !managed {
		return nil
	}
	if err := rejectSymlinkedUploadAncestors(state); err != nil {
		return fmt.Errorf("inspect managed instance state directory: %w", err)
	}
	info, err := os.Lstat(state)
	if err != nil {
		return fmt.Errorf("managed instance state directory %q must already exist and belong to the service user: %w", state, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("managed instance state directory %q is not a directory", state)
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || metadata.Uid != uint32(os.Geteuid()) {
		return fmt.Errorf("managed instance state directory %q must belong to the current service user", state)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("managed instance state directory %q must be private (mode 0700)", state)
	}
	return nil
}
