package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gooru.local/types"
	"gooru.local/internal/config"
)

// SendIPCCommand connects to a TCP port, sends a command, and returns the response.
func SendIPCCommand(port int, command string) (string, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return "", fmt.Errorf("could not connect to mount process: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(command)); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}
	return strings.TrimSpace(response), nil
}

// FindActiveMounts scans the runtime directory for active, healthy mount processes.
func FindActiveMounts() ([]types.MountInfo, error) {
	runDir, err := config.GetRunDirPath()
	if err != nil {
		// If the run dir doesn't exist or can't be created, there can be no mounts.
		// This is not an error in the context of just checking.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not get runtime directory: %w", err)
	}

	var mounts []types.MountInfo
	err = filepath.WalkDir(runDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".json") {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil // Skip unreadable files
			}
			var info types.MountInfo
			if err := json.Unmarshal(data, &info); err != nil {
				return nil // Skip malformed files
			}

			// Health check
			resp, err := SendIPCCommand(info.Port, "PING\n")
			if err == nil && resp == "PONG" {
				mounts = append(mounts, info)
			} else {
				// Stale file, clean it up
				_ = os.Remove(path)
			}
		}
		return nil
	})
	return mounts, err
}

// resolveRemoteTargetPath canonicalizes a path for finding a mount point.
func resolveRemoteTargetPath(path string) (string, error) {
	isDriveLetter := false
	if runtime.GOOS == "windows" {
		if len(path) == 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) {
			isDriveLetter = true
		}
	}

	if isDriveLetter {
		return strings.ToUpper(path), nil
	}

	// For non-drive letters, get the cleaned absolute path.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("could not resolve path '%s': %w", path, err)
	}
	return filepath.Clean(absPath), nil
}

// FindMountByPath finds an active mount process managing a specific path.
func FindMountByPath(path string) (types.MountInfo, error) {
	canonicalPath, err := resolveRemoteTargetPath(path)
	if err != nil {
		return types.MountInfo{}, err
	}

	mounts, err := FindActiveMounts()
	if err != nil {
		return types.MountInfo{}, err
	}

	for _, mount := range mounts {
		if mount.MountPoint == canonicalPath {
			return mount, nil
		}
	}
	return types.MountInfo{}, fmt.Errorf("no active mount found for path: %s", path)
}