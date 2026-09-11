package serve

import (
	"context"
	"sync"

	"gooru.local/types"
)

const uploadAnalysisConcurrency = 4

type uploadAnalysisResult struct {
	Info         types.FileInfo
	Status       types.FileStatus
	AnalysisPath string
	Err          error
}

type indexedUploadAnalysis struct {
	index  int
	result uploadAnalysisResult
}

func (l *GooruLibrary) analyzeUploadedFiles(ctx context.Context, files []StagedUpload, onProgress func(int) error) ([]uploadAnalysisResult, error) {
	return analyzeUploadedFilesConcurrently(ctx, files, uploadAnalysisConcurrency, func(file StagedUpload) uploadAnalysisResult {
		if file.Status == "error" || file.Status == "skipped" {
			return uploadAnalysisResult{}
		}
		analysisPath := file.AnalysisPath
		if analysisPath == "" {
			analysisPath = file.Path
		}
		source, size, modTime, err := l.openUploadAnalysisSource(analysisPath)
		if err != nil {
			return uploadAnalysisResult{AnalysisPath: analysisPath, Err: err}
		}
		info, status, err := l.client.GetFileInfoForSource(file.Path, source, size, modTime)
		closeErr := source.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		return uploadAnalysisResult{Info: info, Status: status, AnalysisPath: analysisPath, Err: err}
	}, onProgress)
}

func analyzeUploadedFilesConcurrently(ctx context.Context, files []StagedUpload, concurrency int, analyze func(StagedUpload) uploadAnalysisResult, onProgress func(int) error) ([]uploadAnalysisResult, error) {
	if len(files) == 0 {
		return []uploadAnalysisResult{}, nil
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(files) {
		concurrency = len(files)
	}

	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	results := make(chan indexedUploadAnalysis)
	var workers sync.WaitGroup
	workers.Add(concurrency)
	for range concurrency {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-workerCtx.Done():
					return
				case index, ok := <-jobs:
					if !ok {
						return
					}
					result := analyze(files[index])
					select {
					case results <- indexedUploadAnalysis{index: index, result: result}:
					case <-workerCtx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range files {
			select {
			case jobs <- index:
			case <-workerCtx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	ordered := make([]uploadAnalysisResult, len(files))
	finished := make([]bool, len(files))
	received := 0
	reported := 0
	var progressErr error
	for item := range results {
		ordered[item.index] = item.result
		finished[item.index] = true
		received++
		previousReported := reported
		for reported < len(finished) && finished[reported] {
			reported++
		}
		if progressErr == nil && onProgress != nil {
			for completed := previousReported + 1; completed <= reported; completed++ {
				if err := onProgress(completed); err != nil {
					progressErr = err
					cancel()
					break
				}
			}
		}
	}
	if progressErr != nil {
		return nil, progressErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if received != len(files) {
		return nil, context.Canceled
	}
	return ordered, nil
}
