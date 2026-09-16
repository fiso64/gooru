package hashing

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestConcurrentlyHashFilesHashesEachDistinctPathOnce(t *testing.T) {
	var mu sync.Mutex
	calls := make(map[string]int)
	hasher := &Hasher{
		hashFile: func(path string) (string, error) {
			mu.Lock()
			calls[path]++
			mu.Unlock()
			return "hash:" + path, nil
		},
	}

	got := hasher.ConcurrentlyHashFiles([]string{"/library/a.jpg", "/library/a.jpg", "/library/b.jpg", "/library/a.jpg", "/library/b.jpg"})

	if len(got) != 2 {
		t.Fatalf("result count = %d, want 2", len(got))
	}
	for _, path := range []string{"/library/a.jpg", "/library/b.jpg"} {
		result, ok := got[path]
		if !ok {
			t.Fatalf("missing result for %q", path)
		}
		if result.Err != nil {
			t.Fatalf("result error for %q: %v", path, result.Err)
		}
		if want := "hash:" + path; result.Hash != want {
			t.Fatalf("hash for %q = %q, want %q", path, result.Hash, want)
		}
		if calls[path] != 1 {
			t.Fatalf("hash calls for %q = %d, want 1", path, calls[path])
		}
	}
}

func TestConcurrentlyHashFilesCompletesBeyondWorkerQueueCapacity(t *testing.T) {
	fileCount := runtime.NumCPU()*4 + 17
	files := make([]string, fileCount)
	for i := range files {
		files[i] = fmt.Sprintf("/library/file-%05d.jpg", i)
	}

	hasher := &Hasher{
		hashFile: func(path string) (string, error) {
			return "hash:" + path, nil
		},
	}

	done := make(chan map[string]Result, 1)
	go func() {
		done <- hasher.ConcurrentlyHashFiles(files)
	}()

	select {
	case got := <-done:
		if len(got) != len(files) {
			t.Fatalf("result count = %d, want %d", len(got), len(files))
		}
		for _, path := range files {
			result, ok := got[path]
			if !ok {
				t.Fatalf("missing result for %q", path)
			}
			if result.Err != nil {
				t.Fatalf("result error for %q: %v", path, result.Err)
			}
			if want := "hash:" + path; result.Hash != want {
				t.Fatalf("hash for %q = %q, want %q", path, result.Hash, want)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent hashing did not make progress beyond the bounded worker queues")
	}
}
