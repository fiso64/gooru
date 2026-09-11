package serve

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"gooru.local/types"
)

func TestAnalyzeUploadedFilesConcurrentlyBoundsWorkersAndPreservesOrder(t *testing.T) {
	files := make([]StagedUpload, 8)
	for i := range files {
		files[i] = StagedUpload{Name: fmt.Sprintf("file-%d.jpg", i)}
	}
	started := make(chan struct{}, uploadAnalysisConcurrency)
	release := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	progress := make([]int, 0, len(files))

	done := make(chan struct{})
	var got []uploadAnalysisResult
	var gotErr error
	go func() {
		got, gotErr = analyzeUploadedFilesConcurrently(context.Background(), files, uploadAnalysisConcurrency, func(file StagedUpload) uploadAnalysisResult {
			current := active.Add(1)
			for {
				observed := maxActive.Load()
				if current <= observed || maxActive.CompareAndSwap(observed, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			index, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file.Name, "file-"), ".jpg"))
			return uploadAnalysisResult{Info: types.FileInfo{Path: fmt.Sprintf("logical-%d", index)}}
		}, func(completed int) error {
			progress = append(progress, completed)
			return nil
		})
		close(done)
	}()

	for range uploadAnalysisConcurrency {
		<-started
	}
	if got := maxActive.Load(); got != uploadAnalysisConcurrency {
		t.Fatalf("max active analyses = %d, want %d", got, uploadAnalysisConcurrency)
	}
	close(release)
	<-done
	if gotErr != nil {
		t.Fatalf("analyze uploads: %v", gotErr)
	}
	if len(got) != len(files) {
		t.Fatalf("results = %d, want %d", len(got), len(files))
	}
	for i := range got {
		if want := fmt.Sprintf("logical-%d", i); got[i].Info.Path != want {
			t.Fatalf("result %d path = %q, want %q", i, got[i].Info.Path, want)
		}
	}
	if len(progress) != len(files) || progress[len(progress)-1] != len(files) {
		t.Fatalf("progress = %v", progress)
	}
}

func TestAnalyzeUploadedFilesConcurrentlyReportsOnlyContiguousCompletedPrefix(t *testing.T) {
	files := []StagedUpload{{Name: "file-0.jpg"}, {Name: "file-1.jpg"}, {Name: "file-2.jpg"}}
	release := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	started := make(chan int, len(files))
	progress := make(chan int, len(files))
	done := make(chan error, 1)

	go func() {
		_, err := analyzeUploadedFilesConcurrently(context.Background(), files, 2, func(file StagedUpload) uploadAnalysisResult {
			index, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file.Name, "file-"), ".jpg"))
			started <- index
			<-release[index]
			return uploadAnalysisResult{}
		}, func(completed int) error {
			progress <- completed
			return nil
		})
		done <- err
	}()

	first := <-started
	second := <-started
	if (first != 0 && first != 1) || (second != 0 && second != 1) || first == second {
		t.Fatalf("initial workers started %d and %d, want 0 and 1", first, second)
	}

	close(release[1])
	if next := <-started; next != 2 {
		t.Fatalf("next analysis index = %d, want 2", next)
	}
	if len(progress) != 0 {
		t.Fatalf("out-of-order completion advanced progress: %d events", len(progress))
	}

	close(release[0])
	if got := <-progress; got != 1 {
		t.Fatalf("first contiguous progress = %d, want 1", got)
	}
	if got := <-progress; got != 2 {
		t.Fatalf("second contiguous progress = %d, want 2", got)
	}

	close(release[2])
	if got := <-progress; got != 3 {
		t.Fatalf("final contiguous progress = %d, want 3", got)
	}
	if err := <-done; err != nil {
		t.Fatalf("analyze uploads: %v", err)
	}
}
