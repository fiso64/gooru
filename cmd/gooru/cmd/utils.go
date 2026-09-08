package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ExpansionResult holds the results of expanding file arguments.
type ExpansionResult struct {
	Found    []string // Absolute paths to files/dirs that exist.
	NotFound []string // Original arguments that did not exist on disk.
}

// expandFileArgs takes a list of arguments (which can be files, directories, or glob patterns)
// and returns a deduplicated, sorted list of absolute file paths, separating found and not-found paths.
func expandFileArgs(args []string) (ExpansionResult, error) {
	seen := make(map[string]struct{})
	result := ExpansionResult{}

	for _, arg := range args {
		// filepath.Match documentation states these are the metacharacters.
		isGlob := strings.ContainsAny(arg, "*?[")

		matches, err := filepath.Glob(arg)
		if err != nil {
			return ExpansionResult{}, err // Invalid glob pattern
		}

		if len(matches) == 0 {
			// If it's not a glob, it's a specific path that doesn't exist.
			if !isGlob {
				result.NotFound = append(result.NotFound, arg)
			}
			// If it *is* a glob, it's just a pattern that matched no files, which is not an error.
			continue
		}

		// If it is a glob or a valid path, process all matches
		for _, match := range matches {
			err := processPath(match, seen, &result.Found)
			if err != nil {
				return ExpansionResult{}, err
			}
		}
	}

	sort.Strings(result.Found)
	sort.Strings(result.NotFound)
	return result, nil
}

// processPath checks if a path is a file or directory and adds it to the list.
// It now assumes that the initial `path` exists.
func processPath(path string, seen map[string]struct{}, files *[]string) error {
	info, err := os.Stat(path)
	if err != nil {
		// This can happen due to a race condition (file deleted between glob and stat)
		// or permission errors. It is correct to propagate this as an error.
		return err
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
