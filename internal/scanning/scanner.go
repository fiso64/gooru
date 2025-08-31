package scanning

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"sync"

	"gooru.local/gooru/internal/hashing"
	"gooru.local/gooru/types"
)

type job struct {
	path string
	info fs.FileInfo
}

type result struct {
	path    string
	info    types.LocationInfo
	err     error
	skipped bool
}

// DirsConcurrently intelligently scans directories, only hashing files whose size matches a known file.
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
			filepath.WalkDir(d, func(path string, de fs.DirEntry, err error) error {
				if err != nil {
					return nil // Skip files we can't access
				}
				if !de.IsDir() {
					info, err := de.Info()
					if err == nil {
						jobs <- job{path: path, info: info}
					}
				}
				return nil
			})
		}(dir)
	}

	// Close jobs channel once all directories have been walked.
	go func() {
		walkWg.Wait()
		close(jobs)
	}()

	// Close results channel once all workers are finished.
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
		// Optionally log res.err if a file failed to hash
	}

	return foundFiles, filesScanned
}

// worker is a worker goroutine that processes scan jobs.
func worker(wg *sync.WaitGroup, jobs <-chan job, results chan<- result, sizeToHashes map[int64][]string, hasher *hashing.Hasher) {
	defer wg.Done()
	for job := range jobs {
		fileSize := job.info.Size()
		if _, ok := sizeToHashes[fileSize]; !ok {
			// This file's size does not match any known file. Skip it entirely.
			results <- result{path: job.path, err: nil, skipped: true}
			continue
		}

		// Use the provided hasher, which is configured according to the database's strategy.
		hash, err := hasher.HashFile(job.path)
		res := result{path: job.path, err: err}
		if err == nil {
			res.info = types.LocationInfo{
				Path:      job.path,
				Hash:      hash,
				Size:      fileSize,
				ModTime:   job.info.ModTime().Unix(),
				Extension: filepath.Ext(job.path),
			}
		}
		results <- res
	}
}