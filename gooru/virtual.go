package gooru

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gooru.local/internal/ipc"
)

// resolveIfVirtual checks if a given absolute path falls within an active mount point.
// If it does, it resolves it to its real path via IPC.
// It returns the real path, a boolean indicating if it was a virtual path, and any error.
func resolveIfVirtual(absPath string) (string, bool, error) {
	mounts, err := ipc.FindActiveMounts()
	if err != nil {
		// This can happen if the ~/.config/gooru/run dir is unreadable.
		// We should not prevent the user from operating on normal files.
		// We will log this if verbose mode is on, but otherwise treat as "no mounts".
		return "", false, nil
	}

	if len(mounts) == 0 {
		return "", false, nil
	}

	for _, mount := range mounts {
		// Check if absPath is the mount point itself or a path inside it.
		if absPath == mount.MountPoint || strings.HasPrefix(absPath, mount.MountPoint+string(filepath.Separator)) {

			// It's inside a mount point. Determine the path relative to the mount root.
			vfsPath := "/"
			if absPath != mount.MountPoint {
				virtualPart := strings.TrimPrefix(absPath, mount.MountPoint+string(filepath.Separator))
				vfsPath = "/" + filepath.ToSlash(virtualPart)
			}

			command := fmt.Sprintf("RESOLVE %s\n", vfsPath)
			response, err := ipc.SendIPCCommand(mount.Port, command)
			if err != nil {
				return "", true, fmt.Errorf("could not communicate with mount at '%s': %w", mount.MountPoint, err)
			}

			parts := strings.SplitN(response, " ", 2)
			status := parts[0]

			if status == "OK" && len(parts) > 1 {
				return parts[1], true, nil
			}

			if status == "ERROR" {
				errMsg := "unknown error"
				if len(parts) > 1 {
					errMsg = parts[1]
				}
				// If the file is not found in the VFS, it's a "not found" error.
				// We can return a standard os.ErrNotExist to be handled by callers.
				if errMsg == "not found" {
					return "", true, os.ErrNotExist
				}
				return "", true, fmt.Errorf("mount at '%s' returned error: %s", mount.MountPoint, errMsg)
			}

			return "", true, fmt.Errorf("received unknown response from mount: %s", response)
		}
	}

	return "", false, nil
}