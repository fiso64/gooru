package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// expandFileArgs takes a list of arguments (which can be files, directories, or glob patterns)
// and returns a deduplicated, sorted list of absolute file paths.
func expandFileArgs(args []string) ([]string, error) {
	seen := make(map[string]struct{})
	var files []string

	for _, arg := range args {
		// filepath.Glob returns the original pattern if it contains no metacharacters,
		// so we don't need to stat it separately first.
		matches, err := filepath.Glob(arg)
		if err != nil {
			return nil, err // Invalid glob pattern
		}

		if len(matches) > 0 {
			// If it is a glob or a valid path, process all matches
			for _, match := range matches {
				err := processPath(match, seen, &files)
				if err != nil {
					return nil, err
				}
			}
		} else {
			// If not a glob or no matches, it might be a path that doesn't exist yet,
			// which is fine for tagging, so we process the arg directly.
			err := processPath(arg, seen, &files)
			if err != nil {
				return nil, err
			}
		}
	}

	sort.Strings(files)
	return files, nil
}

// processPath checks if a path is a file or directory and adds it to the list.
func processPath(path string, seen map[string]struct{}, files *[]string) error {
	info, err := os.Stat(path)
	if err != nil {
		// File might not exist. For tagging, this is a valid case (the service layer
		// will return an os.Stat error). We can ignore the error here.
		return nil
	}

	if info.IsDir() {
		// If it's a directory, walk it
		return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err // Propagate errors from walking
			}
			if !d.IsDir() {
				absPath, err := filepath.Abs(p)
				if err != nil {
					return err
				}
				if _, exists := seen[absPath]; !exists {
					*files = append(*files, absPath)
					seen[absPath] = struct{}{}
				}
			}
			return nil
		})
	}

	// If it's a file
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, exists := seen[absPath]; !exists {
		*files = append(*files, absPath)
		seen[absPath] = struct{}{}
	}
	return nil
}