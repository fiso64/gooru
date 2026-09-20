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

// defaultUploadPath is resolved under the identity running Gooru, never the
// caller's working directory or an arbitrary temporary directory.
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

// prepareDefaultUploadDir is called only after configured protected-path
// validation. A persistent target must be private and immediately writable.
func prepareDefaultUploadDir(path string) error {
    root := filepath.Dir(path)
    for _, dir := range []string{root, path} {
        if err := os.MkdirAll(dir, 0700); err != nil {
            return fmt.Errorf("prepare default upload directory: %w", err)
        }
        info, err := os.Lstat(dir)
        if err != nil {
            return fmt.Errorf("inspect default upload directory: %w", err)
        }
        if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
            return fmt.Errorf("default upload directory %q must be a real directory", dir)
        }
        if err := os.Chmod(dir, 0700); err != nil {
            return fmt.Errorf("make default upload directory private: %w", err)
        }
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
