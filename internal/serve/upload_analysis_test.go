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
	prefixProgress := make([]int, 0, len(files))

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
		}, func(completed, completedPrefix int) error {
			progress = append(progress, completed)
			prefixProgress = append(prefixProgress, completedPrefix)
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
		t.Fatalf("aggregate progress = %v", progress)
	}
	if len(prefixProgress) != len(files) || prefixProgress[len(prefixProgress)-1] != len(files) {
		t.Fatalf("prefix progress = %v", prefixProgress)
	}
}

func TestAnalyzeUploadedFilesConcurrentlyKeepsAggregateProgressAccurateWhenPrefixIsBlocked(t *testing.T) {
	files := []StagedUpload{{Name: "file-0.jpg"}, {Name: "file-1.jpg"}, {Name: "file-2.jpg"}}
	release := []chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
	started := make(chan int, len(files))
	type progressState struct {
		completed int
		prefix    int
	}
	progress := make(chan progressState, len(files))
	done := make(chan error, 1)

	go func() {
		_, err := analyzeUploadedFilesConcurrently(context.Background(), files, 2, func(file StagedUpload) uploadAnalysisResult {
			index, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(file.Name, "file-"), ".jpg"))
			started <- index
			<-release[index]
			return uploadAnalysisResult{}
		}, func(completed, completedPrefix int) error {
			progress <- progressState{completed: completed, prefix: completedPrefix}
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
	if got := <-progress; got != (progressState{completed: 1, prefix: 0}) {
		t.Fatalf("out-of-order progress = %+v, want completed=1 prefix=0", got)
	}
	if next := <-started; next != 2 {
		t.Fatalf("next analysis index = %d, want 2", next)
	}

	close(release[0])
	if got := <-progress; got != (progressState{completed: 2, prefix: 2}) {
		t.Fatalf("gap-closing progress = %+v, want completed=2 prefix=2", got)
	}

	close(release[2])
	if got := <-progress; got != (progressState{completed: 3, prefix: 3}) {
		t.Fatalf("final progress = %+v, want completed=3 prefix=3", got)
	}
	if err := <-done; err != nil {
		t.Fatalf("analyze uploads: %v", err)
	}
}
