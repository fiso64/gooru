from pathlib import Path


def replace(path, old, new, count=1):
    p = Path(path)
    text = p.read_text()
    actual = text.count(old)
    if actual != count:
        raise SystemExit(f"{path}: expected {count} occurrences, found {actual}: {old[:80]!r}")
    p.write_text(text.replace(old, new))


# Raise the streamed multipart/task ceiling to the owner's 1K-file case.
replace("internal/serve/uploads.go", "const maxUploadFiles = 100", "const maxUploadFiles = 1000")

# Analyze read/hash/status work concurrently, but keep all mutations below ordered.
replace(
    "internal/serve/uploads.go",
    '\topaqueStorageByLogical := make(map[string]string, len(files))\n\tfor _, file := range files {',
    '\topaqueStorageByLogical := make(map[string]string, len(files))\n'
    '\tanalyses, err := l.analyzeUploadedFiles(ctx, files, func(completed int) error {\n'
    '\t\tif operationID == "" || !shouldPersistUploadProgress(completed, len(files)) {\n'
    '\t\t\treturn nil\n'
    '\t\t}\n'
    '\t\treturn l.client.SetBackgroundOperationCheckpoint(operationID, backgroundUploadActivatedCheckpoint(activated, len(files), completed))\n'
    '\t})\n'
    '\tif err != nil {\n'
    '\t\treturn UploadImportResponse{}, err\n'
    '\t}\n'
    '\tfor index, file := range files {',
)
replace(
    "internal/serve/uploads.go",
    '\t\tanalysisPath := file.AnalysisPath\n'
    '\t\tif analysisPath == "" {\n'
    '\t\t\tanalysisPath = file.Path\n'
    '\t\t}\n'
    '\t\tanalysisSource, analysisSize, analysisModTime, err := l.openUploadAnalysisSource(analysisPath)\n'
    '\t\tif err != nil {\n'
    '\t\t\tdto.Status = "error"\n'
    '\t\t\tdto.Error = err.Error()\n'
    '\t\t\tresponse.Files = append(response.Files, dto)\n'
    '\t\t\tcontinue\n'
    '\t\t}\n'
    '\t\tinfo, status, err := l.client.GetFileInfoForSource(file.Path, analysisSource, analysisSize, analysisModTime)\n'
    '\t\tcloseErr := analysisSource.Close()\n'
    '\t\tif err == nil && closeErr != nil {\n'
    '\t\t\terr = closeErr\n'
    '\t\t}\n'
    '\t\tif err != nil {\n'
    '\t\t\tdto.Status = "error"\n'
    '\t\t\tdto.Error = err.Error()\n'
    '\t\t\tresponse.Files = append(response.Files, dto)\n'
    '\t\t\tcontinue\n'
    '\t\t}\n',
    '\t\tanalysis := analyses[index]\n'
    '\t\tanalysisPath := analysis.AnalysisPath\n'
    '\t\tif analysis.Err != nil {\n'
    '\t\t\tdto.Status = "error"\n'
    '\t\t\tdto.Error = analysis.Err.Error()\n'
    '\t\t\tresponse.Files = append(response.Files, dto)\n'
    '\t\t\tcontinue\n'
    '\t\t}\n'
    '\t\tinfo, status := analysis.Info, analysis.Status\n',
)

# Add file-level, non-sensitive progress to the producer-owned upload checkpoint.
replace(
    "internal/serve/background_uploads.go",
    'type backgroundUploadCheckpoint struct {\n'
    '\tPhase        string                                  `json:"phase"`\n'
    '\tReplacements []backgroundUploadReplacementCheckpoint `json:"replacements,omitempty"`\n'
    '\tResponse     *UploadImportResponse                    `json:"response,omitempty"`\n'
    '}',
    'type backgroundUploadCheckpoint struct {\n'
    '\tPhase          string                                  `json:"phase"`\n'
    '\tReplacements   []backgroundUploadReplacementCheckpoint `json:"replacements,omitempty"`\n'
    '\tResponse       *UploadImportResponse                    `json:"response,omitempty"`\n'
    '\tFileTotal      int                                      `json:"file_total,omitempty"`\n'
    '\tFilesCompleted int                                      `json:"files_completed,omitempty"`\n'
    '}',
)
replace(
    "internal/serve/background_uploads.go",
    'func backgroundUploadInitialCheckpoint() backgroundUploadCheckpoint {\n'
    '\treturn backgroundUploadCheckpoint{Phase: backgroundUploadPhaseStaged}\n'
    '}\n\n'
    'func backgroundUploadActivatedCheckpoint(activated []activatedSavedReplacement) backgroundUploadCheckpoint {\n'
    '\tcheckpoint := backgroundUploadCheckpoint{Phase: backgroundUploadPhaseActivated}\n'
    '\tcheckpoint.Replacements = backgroundUploadReplacementCheckpoints(activated)\n'
    '\treturn checkpoint\n'
    '}\n\n'
    'func backgroundUploadImportedCheckpoint(activated []activatedSavedReplacement, response UploadImportResponse) backgroundUploadCheckpoint {\n'
    '\tcheckpoint := backgroundUploadCheckpoint{\n'
    '\t\tPhase:        backgroundUploadPhaseImported,\n'
    '\t\tReplacements: backgroundUploadReplacementCheckpoints(activated),\n'
    '\t\tResponse:     &response,\n'
    '\t}\n'
    '\treturn checkpoint\n'
    '}',
    'func backgroundUploadInitialCheckpoint(fileTotal ...int) backgroundUploadCheckpoint {\n'
    '\ttotal := 0\n'
    '\tif len(fileTotal) > 0 {\n'
    '\t\ttotal = fileTotal[0]\n'
    '\t}\n'
    '\treturn backgroundUploadCheckpoint{Phase: backgroundUploadPhaseStaged, FileTotal: total}\n'
    '}\n\n'
    'func backgroundUploadActivatedCheckpoint(activated []activatedSavedReplacement, progress ...int) backgroundUploadCheckpoint {\n'
    '\tcheckpoint := backgroundUploadCheckpoint{Phase: backgroundUploadPhaseActivated}\n'
    '\tcheckpoint.Replacements = backgroundUploadReplacementCheckpoints(activated)\n'
    '\tif len(progress) > 0 {\n'
    '\t\tcheckpoint.FileTotal = progress[0]\n'
    '\t}\n'
    '\tif len(progress) > 1 {\n'
    '\t\tcheckpoint.FilesCompleted = progress[1]\n'
    '\t}\n'
    '\treturn checkpoint\n'
    '}\n\n'
    'func backgroundUploadImportedCheckpoint(activated []activatedSavedReplacement, response UploadImportResponse) backgroundUploadCheckpoint {\n'
    '\ttotal := len(response.Files)\n'
    '\tcheckpoint := backgroundUploadCheckpoint{\n'
    '\t\tPhase:          backgroundUploadPhaseImported,\n'
    '\t\tReplacements:   backgroundUploadReplacementCheckpoints(activated),\n'
    '\t\tResponse:       &response,\n'
    '\t\tFileTotal:      total,\n'
    '\t\tFilesCompleted: total,\n'
    '\t}\n'
    '\treturn checkpoint\n'
    '}\n\n'
    'func shouldPersistUploadProgress(completed, total int) bool {\n'
    '\tif completed <= 0 || total <= 0 {\n'
    '\t\treturn false\n'
    '\t}\n'
    '\tstep := total / 50\n'
    '\tif step < 1 {\n'
    '\t\tstep = 1\n'
    '\t}\n'
    '\treturn completed == total || completed%step == 0\n'
    '}',
)

replace("internal/serve/uploads_durable.go", "backgroundUploadInitialCheckpoint(), taskRequest", "backgroundUploadInitialCheckpoint(len(saved)), taskRequest")
replace("internal/serve/uploads_durable.go", "backgroundOperationDTO(state)", "s.backgroundOperationDTO(state)")
replace("internal/serve/background_upload_worker.go", "backgroundUploadActivatedCheckpoint(activated)", "backgroundUploadActivatedCheckpoint(activated, len(files), 0)")

# Project upload checkpoint progress at the HTTP boundary without changing task lifecycle counters.
p = Path("internal/serve/background_operations.go")
text = p.read_text()
marker = (
    'type backgroundOperationReader interface {\n'
    '\tGetBackgroundOperation(string) (core.BackgroundOperationState, bool, error)\n'
    '\tListBackgroundOperations(core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error)\n'
    '\tCancelBackgroundOperation(string) (bool, error)\n'
    '\tGetBackgroundOperationResult(string, any) (bool, error)\n'
    '}\n'
)
if marker not in text:
    raise SystemExit("background operation reader marker missing")
text = text.replace(marker, marker + '\ntype backgroundOperationCheckpointReader interface {\n\tGetBackgroundOperationCheckpoint(string, any) (bool, error)\n}\n', 1)
text = text.replace("dto := backgroundOperationDTO(operation)", "dto := s.backgroundOperationDTO(operation)")
text = text.replace("items = append(items, backgroundOperationDTO(operation))", "items = append(items, s.backgroundOperationDTO(operation))")
text = text.replace("writeJSON(w, http.StatusAccepted, backgroundOperationDTO(operation))", "writeJSON(w, http.StatusAccepted, s.backgroundOperationDTO(operation))")
pure = 'func backgroundOperationDTO(operation core.BackgroundOperationState) BackgroundOperationDTO {\n\treturn BackgroundOperationDTO{ID: operation.ID, Kind: operation.Kind, Status: operation.Status, ProgressTotal: operation.ProgressTotal, ProgressCompleted: operation.ProgressCompleted, ProgressFailed: operation.ProgressFailed, CreatedAt: operation.CreatedAt, StartedAt: operation.StartedAt, FinishedAt: operation.FinishedAt, ErrorCode: operation.ErrorCode, ErrorMessage: operation.ErrorMessage}\n}\n'
if pure not in text:
    raise SystemExit("background operation DTO marker missing")
projection = pure + '''
func (s *Server) backgroundOperationDTO(operation core.BackgroundOperationState) BackgroundOperationDTO {
	dto := backgroundOperationDTO(operation)
	if operation.Kind != backgroundUploadImportOperationKind || s.backgroundOperations == nil {
		return dto
	}
	reader, ok := s.backgroundOperations.(backgroundOperationCheckpointReader)
	if !ok {
		return dto
	}
	var checkpoint backgroundUploadCheckpoint
	found, err := reader.GetBackgroundOperationCheckpoint(operation.ID, &checkpoint)
	if err != nil || !found || checkpoint.FileTotal <= 0 {
		return dto
	}
	total := int64(checkpoint.FileTotal)
	completed := int64(checkpoint.FilesCompleted)
	if operation.Status == core.BackgroundWorkCompleted {
		completed = total
	}
	if completed < 0 {
		completed = 0
	}
	if completed > total {
		completed = total
	}
	dto.ProgressTotal = total
	dto.ProgressCompleted = completed
	return dto
}
'''
text = text.replace(pure, projection, 1)
p.write_text(text)

Path("internal/serve/upload_analysis.go").write_text('''package serve

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
	completed := 0
	var progressErr error
	for item := range results {
		ordered[item.index] = item.result
		completed++
		if progressErr == nil && onProgress != nil {
			if err := onProgress(completed); err != nil {
				progressErr = err
				cancel()
			}
		}
	}
	if progressErr != nil {
		return nil, progressErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if completed != len(files) {
		return nil, context.Canceled
	}
	return ordered, nil
}
''')

Path("internal/serve/upload_analysis_test.go").write_text('''package serve

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
''')

p = Path("internal/serve/background_uploads_test.go")
text = p.read_text()
text += '''

func TestBackgroundUploadTaskAcceptsOneThousandFilesAndRejectsMore(t *testing.T) {
	files := make([]savedUpload, maxUploadFiles)
	for i := range files {
		files[i] = savedUpload{name: "a.jpg", path: "/uploads/a.jpg", targetID: "default"}
	}
	if _, err := backgroundUploadTaskRequest("operation-1000", files, nil); err != nil {
		t.Fatalf("1K upload task rejected: %v", err)
	}
	files = append(files, savedUpload{name: "overflow.jpg", path: "/uploads/overflow.jpg", targetID: "default"})
	if _, err := backgroundUploadTaskRequest("operation-1001", files, nil); err == nil {
		t.Fatal("upload task above the bounded 1K ceiling unexpectedly accepted")
	}
}

func TestBackgroundUploadCheckpointCarriesFileProgress(t *testing.T) {
	checkpoint := backgroundUploadActivatedCheckpoint(nil, 1000, 420)
	if checkpoint.FileTotal != 1000 || checkpoint.FilesCompleted != 420 {
		t.Fatalf("checkpoint progress = %+v", checkpoint)
	}
	response := UploadImportResponse{Files: make([]UploadedFileDTO, 1000)}
	checkpoint = backgroundUploadImportedCheckpoint(nil, response)
	if checkpoint.FileTotal != 1000 || checkpoint.FilesCompleted != 1000 {
		t.Fatalf("terminal checkpoint progress = %+v", checkpoint)
	}
}
'''
p.write_text(text)

p = Path("internal/serve/background_operations_test.go")
text = p.read_text()
text = text.replace(
    '\tresults     map[string]json.RawMessage\n\tlistOptions core.BackgroundOperationListOptions',
    '\tresults     map[string]json.RawMessage\n\tcheckpoints map[string]backgroundUploadCheckpoint\n\tlistOptions core.BackgroundOperationListOptions',
    1,
)
method_marker = 'func (f *fakeBackgroundOperationReader) GetBackgroundOperationResult(id string, destination any) (bool, error) {'
if method_marker not in text:
    raise SystemExit("fake reader result method marker missing")
checkpoint_method = '''func (f *fakeBackgroundOperationReader) GetBackgroundOperationCheckpoint(id string, destination any) (bool, error) {
	checkpoint, ok := f.checkpoints[id]
	if !ok {
		return false, nil
	}
	*(destination.(*backgroundUploadCheckpoint)) = checkpoint
	return true, nil
}

'''
text = text.replace(method_marker, checkpoint_method + method_marker, 1)
text += '''

func TestHandleOperationsProjectsUploadFileProgressFromCheckpoint(t *testing.T) {
	createdAt := time.Now().UTC()
	reader := &fakeBackgroundOperationReader{
		operations: []core.BackgroundOperationState{{
			ID: "operation-upload-progress", Kind: backgroundUploadImportOperationKind, Visible: true,
			Status: core.BackgroundWorkRunning, ProgressTotal: 1, CreatedAt: createdAt,
		}},
		checkpoints: map[string]backgroundUploadCheckpoint{
			"operation-upload-progress": {Phase: backgroundUploadPhaseActivated, FileTotal: 1000, FilesCompleted: 420},
		},
	}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload BackgroundOperationListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ProgressTotal != 1000 || payload.Items[0].ProgressCompleted != 420 {
		t.Fatalf("projected upload progress = %+v", payload.Items)
	}
}
'''
p.write_text(text)
