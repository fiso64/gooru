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
		if pathsOverlapForConfig(targetPath, candidatePath) {
			return fmt.Errorf("uploads target %q path overlaps protected %s path", target.ID, candidate.name)
		}
	}
	return nil
}

func pathsOverlapForConfig(left, right string) bool {
	leftAbs, err := filepath.Abs(strings.TrimSpace(left))
	if err != nil {
		return false
	}
	rightAbs, err := filepath.Abs(strings.TrimSpace(right))
	if err != nil {
		return false
	}
	leftResolved, ok := resolvedContainmentPath(leftAbs)
	if !ok {
		return false
	}
	rightResolved, ok := resolvedContainmentPath(rightAbs)
	if !ok {
		return false
	}
	return pathContainsOrEquals(leftResolved, rightResolved) || pathContainsOrEquals(rightResolved, leftResolved)
}

func pathContainsOrEquals(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
