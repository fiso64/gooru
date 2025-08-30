package hashing

import (
	"runtime"
	"sync"
)

// Job represents a file to be hashed.
type Job struct {
	FilePath string
}

// Result holds the outcome of a hashing operation.
type Result struct {
	FilePath string
	Hash     string
	Err      error
}

// ConcurrentlyHashFiles takes a list of file paths that need hashing and processes them in parallel.
func ConcurrentlyHashFiles(filesToHash []string) map[string]Result {
	if len(filesToHash) == 0 {
		return make(map[string]Result)
	}

	jobs := make(chan Job, len(filesToHash))
	results := make(chan Result, len(filesToHash))

	numWorkers := runtime.NumCPU()
	if len(filesToHash) < numWorkers {
		numWorkers = len(filesToHash)
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go worker(&wg, jobs, results)
	}

	// Send all jobs to the workers
	for _, f := range filesToHash {
		jobs <- Job{FilePath: f}
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect all results into a map for easy lookup
	resultMap := make(map[string]Result, len(filesToHash))
	for r := range results {
		resultMap[r.FilePath] = r
	}
	return resultMap
}

// worker is a goroutine that reads jobs, hashes files, and sends results.
func worker(wg *sync.WaitGroup, jobs <-chan Job, results chan<- Result) {
	defer wg.Done()
	for job := range jobs {
		hash, err := HashFile(job.FilePath)
		results <- Result{
			FilePath: job.FilePath,
			Hash:     hash,
			Err:      err,
		}
	}
}