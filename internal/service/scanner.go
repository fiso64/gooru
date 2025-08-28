package service

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"sync"

	"gooru.local/gooru/internal/hashing"
	"gooru.local/gooru/internal/types"
)

type scanJob struct {
	path string
	info fs.FileInfo
}

type scanResult struct {
	path    string
	info    types.LocationInfo
	err     error
	skipped bool
}

// scanDirsConcurrently intelligently scans directories, only hashing files whose size matches a known file.
func (s *Service) scanDirsConcurrently(dirs []string, sizeToHashes map[int64][]string) (map[string]types.LocationInfo, int) {
	jobs := make(chan scanJob)
	results := make(chan scanResult)

	var wg sync.WaitGroup
	numWorkers := runtime.NumCPU()
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go s.scanWorker(&wg, jobs, results, sizeToHashes)
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
						jobs <- scanJob{path: path, info: info}
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

// scanWorker is a worker goroutine that processes scan jobs.
func (s *Service) scanWorker(wg *sync.WaitGroup, jobs <-chan scanJob, results chan<- scanResult, sizeToHashes map[int64][]string) {
	defer wg.Done()
	for job := range jobs {
		fileSize := job.info.Size()
		if _, ok := sizeToHashes[fileSize]; !ok {
			// This file's size does not match any known file. Skip it entirely.
			results <- scanResult{path: job.path, err: nil, skipped: true}
			continue
		}

		// Only hash files that could possibly be a match.
		hash, err := hashing.HashFile(job.path)
		result := scanResult{path: job.path, err: err}
		if err == nil {
			result.info = types.LocationInfo{
				Hash:    hash,
				Size:    fileSize,
				ModTime: job.info.ModTime().Unix(),
			}
		}
		results <- result
	}
}