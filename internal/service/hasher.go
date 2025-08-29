package service

import (
	"runtime"
	"sync"

	"gooru.local/gooru/internal/hashing"
)

type hashJob struct {
	filePath string
}

type hashResult struct {
	filePath string
	hash     string
	err      error
}

// concurrentlyHashFiles takes a list of file paths that need hashing and processes them in parallel.
func concurrentlyHashFiles(filesToHash []string) map[string]hashResult {
	if len(filesToHash) == 0 {
		return make(map[string]hashResult)
	}

	jobs := make(chan hashJob, len(filesToHash))
	results := make(chan hashResult, len(filesToHash))

	numWorkers := runtime.NumCPU()
	if len(filesToHash) < numWorkers {
		numWorkers = len(filesToHash)
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go hashWorker(&wg, jobs, results)
	}

	// Send all jobs to the workers
	for _, f := range filesToHash {
		jobs <- hashJob{filePath: f}
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect all results into a map for easy lookup
	resultMap := make(map[string]hashResult, len(filesToHash))
	for r := range results {
		resultMap[r.filePath] = r
	}
	return resultMap
}

// hashWorker is a goroutine that reads jobs, hashes files, and sends results.
func hashWorker(wg *sync.WaitGroup, jobs <-chan hashJob, results chan<- hashResult) {
	defer wg.Done()
	for job := range jobs {
		hash, err := hashing.HashFile(job.filePath)
		results <- hashResult{
			filePath: job.filePath,
			hash:     hash,
			err:      err,
		}
	}
}