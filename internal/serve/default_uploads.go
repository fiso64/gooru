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


 // Use the same per-instance state root for packaged CLI commands and systemd.
 // systemd exports STATE_DIRECTORY; the packaged CLI does not.
 func defaultUploadPathForConfig(configPath string) (string, error) {
     if runtime.GOOS == "linux" && strings.TrimSpace(os.Getenv("STATE_DIRECTORY")) == "" {
         if stateDir, ok := managedStateDirectoryFromConfig(configPath); ok {
             return filepath.Join(stateDir, "uploads"), nil
         }
     }
     return defaultUploadPath()
 }

 func managedStateDirectoryFromConfig(configPath string) (string, bool) {
     if !filepath.IsAbs(configPath) { return "", false }
     relative, err := filepath.Rel("/etc/gooru", filepath.Clean(configPath))
     if err != nil { return "", false }
     parts := strings.Split(relative, string(filepath.Separator))
     if len(parts) != 2 || parts[1] != "serve.yaml" { return "", false }
     name := parts[0]
     if len(name) == 0 || len(name) > 24 || strings.ToLower(name) != name || !validUploadTargetID(name) {
         return "", false
     }
     return filepath.Join("/var/lib", "gooru-" + name), true
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
