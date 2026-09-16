package hashing

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"gooru.local/internal/filesource"
	"gooru.local/internal/hashing/hashes"
	"gooru.local/types"
)

// hashFunc is a function signature for any file hashing implementation.
type hashFunc func(string) (string, error)
type hashSourceFunc func(io.ReaderAt, int64) (string, error)

// FileMetadata describes the logical plaintext file independent of its physical
// storage representation.
type FileMetadata struct {
	Size    int64
	ModTime time.Time
}

// Hasher is configured with a specific hashing strategy. When a logical source
// resolver is installed, all path-based hashing is transparently routed through
// the resolved plaintext source so callers cannot hash encrypted container
// bytes by choosing the convenient path API.
type Hasher struct {
	hashFile   hashFunc
	hashSource hashSourceFunc
	sources    *filesource.Resolver
}

// NewHasher creates a new Hasher based on the provided strategy.
func NewHasher(strategy types.HashingStrategy) (*Hasher, error) {
	var hf hashFunc
	var hs hashSourceFunc
	switch strategy {
	case types.StrategyPartial:
		hf = hashes.HashFile
		hs = hashes.HashSource
	case types.StrategyFull:
		hf = hashes.HashFileFull
		hs = hashes.HashSourceFull
	default:
		return nil, fmt.Errorf("unknown hashing strategy: %s", strategy)
	}
	return &Hasher{hashFile: hf, hashSource: hs}, nil
}

// SetSourceResolver installs the storage policy used by HashFile and concurrent
// path hashing. The resolver belongs to the composition layer, not feature code.
func (h *Hasher) SetSourceResolver(resolver *filesource.Resolver) {
	h.sources = resolver
}

// FileMetadata returns logical plaintext metadata for a path. With the ordinary
// filesystem resolver this remains a stat-only operation so large scans do not
// open every file simply to compare size and modification time.
func (h *Hasher) FileMetadata(filePath string) (FileMetadata, error) {
	if h.sources == nil {
		info, err := os.Stat(filePath)
		if err != nil {
			return FileMetadata{}, err
		}
		return FileMetadata{Size: info.Size(), ModTime: info.ModTime()}, nil
	}
	metadata, err := h.sources.Metadata(filePath)
	if err != nil {
		return FileMetadata{}, err
	}
	return FileMetadata{Size: metadata.Size, ModTime: metadata.ModTime}, nil
}

// HashFile computes a hash for the logical file at filePath using the configured
// strategy. With a source resolver configured, protected managed paths are
// decrypted and hashed as plaintext rather than as their container bytes.
func (h *Hasher) HashFile(filePath string) (string, error) {
	if h.sources == nil {
		return h.hashFile(filePath)
	}
	source, err := h.sources.Open(filePath)
	if err != nil {
		return "", err
	}
	defer source.Close()
	return h.hashSource(source, source.Size())
}

// HashSource computes the same content identity from an arbitrary random-access
// source. This is the path-independent primitive used by protected storage.
func (h *Hasher) HashSource(source io.ReaderAt, size int64) (string, error) {
	return h.hashSource(source, size)
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

	uniqueFiles := make([]string, 0, len(filesToHash))
	seen := make(map[string]struct{}, len(filesToHash))
	for _, filePath := range filesToHash {
		if _, ok := seen[filePath]; ok {
			continue
		}
		seen[filePath] = struct{}{}
		uniqueFiles = append(uniqueFiles, filePath)
	}

	jobs := make(chan Job, len(uniqueFiles))
	results := make(chan Result, len(uniqueFiles))

	numWorkers := runtime.NumCPU()
	if len(uniqueFiles) < numWorkers {
		numWorkers = len(uniqueFiles)
	}

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go h.worker(&wg, jobs, results)
	}

	// Send all jobs to the workers
	for _, f := range uniqueFiles {
		jobs <- Job{FilePath: f}
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect all results into a map for easy lookup
	resultMap := make(map[string]Result, len(uniqueFiles))
	for r := range results {
		resultMap[r.FilePath] = r
	}
	return resultMap
}

// worker is a goroutine that reads jobs, hashes files, and sends results.
func (h *Hasher) worker(wg *sync.WaitGroup, jobs <-chan Job, results chan<- Result) {
	defer wg.Done()
	for job := range jobs {
		hash, err := h.HashFile(job.FilePath)
		results <- Result{
			FilePath: job.FilePath,
			Hash:     hash,
			Err:      err,
		}
	}
}
