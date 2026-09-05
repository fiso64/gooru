package scanning

import (
	"path/filepath"
	"runtime"
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
func DirsConcurrently(dirs []string, sizeToHashes map[int64][]string, hasher *hashing.Hasher) (map[string]types.LocationInfo, int) {
	jobs := make(chan job)
	results := make(chan result)

	var wg sync.WaitGroup
	numWorkers := runtime.NumCPU()
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker(&wg, jobs, results, sizeToHashes, hasher)
	}

	var walkWg sync.WaitGroup
	for _, dir := range dirs {
		walkWg.Add(1)
		go func(d string) {
			defer walkWg.Done()
			_ = filepath.WalkDir(d, func(path string, de interface{ IsDir() bool }, err error) error {
				if err != nil {
					return nil // Skip files we can't access.
				}
				if !de.IsDir() {
					jobs <- job{path: path}
				}
				return nil
			})
		}(dir)
	}

	go func() {
		walkWg.Wait()
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	foundFiles := make(map[string]types.LocationInfo)
	filesScanned := 0
	for res := range results {
		filesScanned++
		if res.err == nil && !res.skipped {
			foundFiles[res.path] = res.info
		}
	}

	return foundFiles, filesScanned
}

func worker(wg *sync.WaitGroup, jobs <-chan job, results chan<- result, sizeToHashes map[int64][]string, hasher *hashing.Hasher) {
	defer wg.Done()
	for job := range jobs {
		metadata, err := hasher.FileMetadata(job.path)
		if err != nil {
			results <- result{path: job.path, err: err}
			continue
		}
		if _, ok := sizeToHashes[metadata.Size]; !ok {
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
