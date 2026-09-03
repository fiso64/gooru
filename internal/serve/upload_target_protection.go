package serve

import (
	"fmt"
	"path/filepath"
	"strings"
)

type protectedConfigPath struct {
	name string
	path string
}

func (cfg Config) validateUploadTargetProtection(target UploadTarget) error {
	targetPath := strings.TrimSpace(target.Path)
	if targetPath == "" || !filepath.IsAbs(targetPath) {
		return nil
	}

	protected := []protectedConfigPath{
		{name: "database.path", path: cfg.Database.Path},
		{name: "encryption.key_file", path: cfg.Encryption.KeyFile},
		{name: "media.cache_dir", path: cfg.Media.CacheDir},
		{name: "server.frontend_dir", path: cfg.Server.FrontendDir},
	}
	if filepath.IsAbs(strings.TrimSpace(cfg.Tools.FFmpegPath)) {
		protected = append(protected, protectedConfigPath{name: "tools.ffmpeg_path", path: cfg.Tools.FFmpegPath})
	}
	if filepath.IsAbs(strings.TrimSpace(cfg.Tools.FFprobePath)) {
		protected = append(protected, protectedConfigPath{name: "tools.ffprobe_path", path: cfg.Tools.FFprobePath})
	}

	for _, candidate := range protected {
		candidatePath := strings.TrimSpace(candidate.path)
		if candidatePath == "" {
			continue
		}
		overlaps, err := pathsOverlapForConfig(targetPath, candidatePath)
		if err != nil {
			return fmt.Errorf("uploads target %q cannot be safely compared with protected %s path: %w", target.ID, candidate.name, err)
		}
		if overlaps {
			return fmt.Errorf("uploads target %q path overlaps protected %s path", target.ID, candidate.name)
		}
	}
	return nil
}

func pathsOverlapForConfig(left, right string) (bool, error) {
	leftResolved, err := resolvedConfigContainmentPath(left)
	if err != nil {
		return false, err
	}
	rightResolved, err := resolvedConfigContainmentPath(right)
	if err != nil {
		return false, err
	}
	return pathContainsOrEquals(leftResolved, rightResolved) || pathContainsOrEquals(rightResolved, leftResolved), nil
}

func resolvedConfigContainmentPath(path string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", fmt.Errorf("make path absolute: %w", err)
	}
	resolved, ok := resolvedContainmentPath(absolute)
	if !ok {
		return "", fmt.Errorf("cannot safely resolve filesystem path")
	}
	return resolved, nil
}

func pathContainsOrEquals(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
