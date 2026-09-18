package scanning

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gooru.local/internal/hashing"
	"gooru.local/types"
)

type job struct {
	path string
}

type result struct {
	path    string
	info    types.LocationInfo
	err     error
	skipped bool
}

// DirsConcurrently intelligently scans directories, only hashing files whose
// logical plaintext size matches a known file. Physical encrypted-container
// size is never used for identity filtering.
func DirsConcurrently(dirs []string, knownSizes map[int64]struct{}, hasher *hashing.Hasher) (map[string]types.LocationInfo, int, error) {
	dirs = PruneRedundantDirs(dirs)
	jobs := make(chan job)
	results := make(chan result)
	walkErrs := make(chan error, len(dirs))

	var wg sync.WaitGroup
	numWorkers := runtime.NumCPU()
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker(&wg, jobs, results, knownSizes, hasher)
	}

	var walkWg sync.WaitGroup
	for _, dir := range dirs {
		walkWg.Add(1)
		go func(d string) {
			defer walkWg.Done()
			if err := filepath.WalkDir(d, func(path string, de fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !de.IsDir() {
					jobs <- job{path: path}
				}
				return nil
			}); err != nil {
				walkErrs <- fmt.Errorf("walk %q: %w", d, err)
			}
		}(dir)
	}

	go func() {
		walkWg.Wait()
		close(jobs)
		close(walkErrs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	foundFiles := make(map[string]types.LocationInfo)
	filesScanned := 0
	var firstErr error
	for res := range results {
		filesScanned++
		if res.err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("inspect %q: %w", res.path, res.err)
			}
			continue
		}
		if !res.skipped {
			foundFiles[res.path] = res.info
		}
	}
	for err := range walkErrs {
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return nil, filesScanned, firstErr
	}

	return foundFiles, filesScanned, nil
}

// PruneRedundantDirs removes exact duplicate and nested scan roots while preserving surviving caller order.
func PruneRedundantDirs(dirs []string) []string {
	if len(dirs) < 2 {
		return dirs
	}

	kept := make([]string, 0, len(dirs))
	for i, dir := range dirs {
		clean := filepath.Clean(dir)
		redundant := false
		for j, other := range dirs {
			if i == j {
				continue
			}
			otherClean := filepath.Clean(other)
			if clean == otherClean {
				if j < i {
					redundant = true
					break
				}
				continue
			}
			rel, err := filepath.Rel(otherClean, clean)
			if err != nil {
				continue
			}
			if rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				redundant = true
				break
			}
		}
		if !redundant {
			kept = append(kept, dir)
		}
	}
	return kept
}

func worker(wg *sync.WaitGroup, jobs <-chan job, results chan<- result, knownSizes map[int64]struct{}, hasher *hashing.Hasher) {
	defer wg.Done()
	for job := range jobs {
		metadata, err := hasher.FileMetadata(job.path)
		if err != nil {
			results <- result{path: job.path, err: err}
			continue
		}
		if _, ok := knownSizes[metadata.Size]; !ok {
			results <- result{path: job.path, skipped: true}
			continue
		}

		hash, err := hasher.HashFile(job.path)
		res := result{path: job.path, err: err}
		if err == nil {
			res.info = types.LocationInfo{
				Path:      job.path,
				Hash:      hash,
				Size:      metadata.Size,
				ModTime:   metadata.ModTime.Unix(),
				Extension: filepath.Ext(job.path),
			}
		}
		results <- res
	}
}
