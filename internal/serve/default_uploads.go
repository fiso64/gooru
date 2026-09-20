package serve

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// defaultUploadPath resolves the state of the identity running Gooru.
func defaultUploadPath() (string, error) {
	if state := strings.TrimSpace(os.Getenv("STATE_DIRECTORY")); state != "" && runtime.GOOS == "linux" {
		state = strings.Split(state, ":")[0]
		if !filepath.IsAbs(state) {
			return "", fmt.Errorf("STATE_DIRECTORY must contain an absolute service state directory")
		}
		return filepath.Join(filepath.Clean(state), "uploads"), nil
	}
	if runtime.GOOS == "windows" {
		data := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if !filepath.IsAbs(data) {
			return "", fmt.Errorf("LOCALAPPDATA must identify an absolute per-user data directory for default uploads")
		}
		return filepath.Join(data, "Gooru", "uploads"), nil
	}

	identity, err := user.LookupId(strconv.Itoa(os.Geteuid()))
	if err != nil || identity.HomeDir == "" || !filepath.IsAbs(identity.HomeDir) {
		return "", fmt.Errorf("resolve current Gooru user home for default uploads: %v", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(identity.HomeDir, "Library", "Application Support", "Gooru", "uploads"), nil
	}
	data := strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	if data == "" {
		data = filepath.Join(identity.HomeDir, ".local", "share")
	}
	if !filepath.IsAbs(data) {
		return "", fmt.Errorf("XDG_DATA_HOME must be absolute for default uploads")
	}
	return filepath.Join(data, "gooru", "uploads"), nil
}

// systemd exports STATE_DIRECTORY; the packaged CLI uses its config path.
func defaultUploadPathForConfig(configPath string) (string, error) {
	if runtime.GOOS == "linux" && strings.TrimSpace(os.Getenv("STATE_DIRECTORY")) == "" {
		if stateDir, ok := managedStateDirectoryFromConfig(configPath); ok {
			return filepath.Join(stateDir, "uploads"), nil
		}
	}
	return defaultUploadPath()
}

func managedStateDirectoryFromConfig(configPath string) (string, bool) {
	if !filepath.IsAbs(configPath) {
		return "", false
	}
	relative, err := filepath.Rel("/etc/gooru", filepath.Clean(configPath))
	if err != nil {
		return "", false
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 2 || parts[1] != "serve.yaml" {
		return "", false
	}
	name := parts[0]
	if len(name) == 0 || len(name) > 24 || strings.ToLower(name) != name || !validUploadTargetID(name) {
		return "", false
	}
	return filepath.Join("/var/lib", "gooru-"+name), true
}

// rejectSymlinkedUploadAncestors checks *existing* components before any
// directory creation. Neither a parent symlink nor the target may redirect
// a default upload into unrelated application data.
func rejectSymlinkedUploadAncestors(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("default upload target %q must be absolute", path)
	}
	for candidate := filepath.Clean(path); ; candidate = filepath.Dir(candidate) {
		info, err := os.Lstat(candidate)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("default upload path %q contains a symlink at %q", path, candidate)
			}
			if !info.IsDir() {
				return fmt.Errorf("default upload path %q contains a non-directory at %q", path, candidate)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect default upload path %q: %w", candidate, err)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
	}
	return nil
}

// prepareDefaultUploadDir never changes the permissions of an existing parent
// (including systemd-owned instance state or a shared XDG data directory).
func prepareDefaultUploadDir(path string) error {
	if err := rejectSymlinkedUploadAncestors(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("prepare default upload parent: %w", err)
	}
	if err := rejectSymlinkedUploadAncestors(path); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("prepare default upload directory: %w", err)
	}
	if err := rejectSymlinkedUploadAncestors(path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0700); err != nil {
		return fmt.Errorf("make default upload directory private: %w", err)
	}
	file, err := os.CreateTemp(path, ".gooru-writable-*")
	if err != nil {
		return fmt.Errorf("default upload directory is not writable: %w", err)
	}
	name := file.Name()
	closeErr := file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil || removeErr != nil {
		return fmt.Errorf("cannot verify default upload directory writable and clean: close=%v remove=%v", closeErr, removeErr)
	}
	return nil
}
