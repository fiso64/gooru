package hashing

import (
	"fmt"
	"runtime"
	"sync"

	"gooru.local/types"
	"gooru.local/internal/hashing/hashes"
)

// hashFunc is a function signature for any file hashing implementation.
type hashFunc func(string) (string, error)

// Hasher is configured with a specific hashing strategy.
type Hasher struct {
	hashFile hashFunc
}

// NewHasher creates a new Hasher based on the provided strategy.
func NewHasher(strategy types.HashingStrategy) (*Hasher, error) {
	var hf hashFunc
	switch strategy {
	case types.StrategyPartial:
		hf = hashes.HashFile
	case types.StrategyFull:
		hf = hashes.HashFileFull
	default:
		return nil, fmt.Errorf("unknown hashing strategy: %s", strategy)
	}
	return &Hasher{hashFile: hf}, nil
}

// HashFile computes a hash for the given file path using the configured strategy.
func (h *Hasher) HashFile(filePath string) (string, error) {
	return h.hashFile(filePath)
}

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
func (h *Hasher) ConcurrentlyHashFiles(filesToHash []string) map[string]Result {
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
		go h.worker(&wg, jobs, results)
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
func (h *Hasher) worker(wg *sync.WaitGroup, jobs <-chan Job, results chan<- Result) {
	defer wg.Done()
	for job := range jobs {
		hash, err := h.hashFile(job.FilePath)
		results <- Result{
			FilePath: job.FilePath,
			Hash:     hash,
			Err:      err,
		}
	}
}