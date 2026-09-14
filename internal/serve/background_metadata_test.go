package serve

import (
	"context"
	"errors"
	"io"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type countingMediaMetadataProvider struct {
	calls int
}

func (p *countingMediaMetadataProvider) Metadata(context.Context, types.FileInfo, string, string) (MediaMetadata, error) {
	p.calls++
	width := 1
	return MediaMetadata{ImageWidth: &width}, nil
}

func (p *countingMediaMetadataProvider) MetadataFromSource(context.Context, types.FileInfo, io.ReaderAt, int64, string, string) (MediaMetadata, error) {
	p.calls++
	width := 1
	return MediaMetadata{ImageWidth: &width}, nil
}

type recordingBackgroundUploadStore struct {
	events      *[]string
	resultCalls int
}

func (s *recordingBackgroundUploadStore) GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error) {
	return core.BackgroundOperationState{}, false, nil
}

func (s *recordingBackgroundUploadStore) GetBackgroundOperationCheckpoint(string, any) (bool, error) {
	return false, nil
}

func (s *recordingBackgroundUploadStore) SetBackgroundOperationCheckpoint(string, any) error {
	return nil
}

func (s *recordingBackgroundUploadStore) SetBackgroundOperationResult(string, any) error {
	s.resultCalls++
	if s.events != nil {
		*s.events = append(*s.events, "result")
	}
	return nil
}

type recordingBackgroundTaskEnqueuer struct {
	events  *[]string
	request core.BackgroundTaskRequest
	err     error
}

func (e *recordingBackgroundTaskEnqueuer) EnqueueBackgroundTask(request core.BackgroundTaskRequest) (core.BackgroundTask, bool, error) {
	e.request = request
	if e.events != nil {
		*e.events = append(*e.events, "enqueue")
	}
	return core.BackgroundTask{}, false, e.err
}

func TestDeferredUploadMediaMetadataSkipsSynchronousExtraction(t *testing.T) {
	provider := &countingMediaMetadataProvider{}
	library := &GooruLibrary{}
	file := types.FileInfo{Path: "/library/photo.jpg"}

	metadata, err := library.importedMediaMetadata(withDeferredUploadMediaMetadata(context.Background()), provider, file, "", "image/jpeg", "photo")
	if err != nil {
		t.Fatalf("deferred metadata: %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("durable upload metadata provider calls = %d, want 0", provider.calls)
	}
	if metadata.ImageWidth != nil {
		t.Fatalf("deferred metadata should be empty, got %+v", metadata)
	}

	metadata, err = library.importedMediaMetadata(context.Background(), provider, file, "", "image/jpeg", "photo")
	if err != nil {
		t.Fatalf("direct metadata: %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("direct upload metadata provider calls = %d, want 1", provider.calls)
	}
	if metadata.ImageWidth == nil || *metadata.ImageWidth != 1 {
		t.Fatalf("direct metadata = %+v, want image width", metadata)
	}
}

func TestBackgroundMediaMetadataTaskBelongsToUploadOperation(t *testing.T) {
	request := backgroundMediaMetadataTaskRequest("operation-id")
	if request.OperationID != "operation-id" || request.SubjectKind != "operation" || request.SubjectID != "operation-id" {
		t.Fatalf("unexpected finalizer ownership: %+v", request)
	}
	if request.Kind != backgroundMediaMetadataTaskKind || request.ResourceClass != backgroundThumbnailResourceClass {
		t.Fatalf("unexpected finalizer routing: %+v", request)
	}
	if request.DedupeKey != "operation-id:metadata-finalize" {
		t.Fatalf("unexpected finalizer dedupe key: %q", request.DedupeKey)
	}
}

func TestBackgroundUploadFinalizerEnqueuesBeforeResultPublication(t *testing.T) {
	events := []string{}
	store := &recordingBackgroundUploadStore{events: &events}
	tasks := &recordingBackgroundTaskEnqueuer{events: &events}
	finalizing := backgroundUploadFinalizingStore{backgroundUploadWorkerStore: store, tasks: tasks}

	if err := finalizing.SetBackgroundOperationResult("operation-id", UploadImportResponse{}); err != nil {
		t.Fatalf("publish upload result: %v", err)
	}
	if len(events) != 2 || events[0] != "enqueue" || events[1] != "result" {
		t.Fatalf("finalization order = %v, want [enqueue result]", events)
	}
	if store.resultCalls != 1 {
		t.Fatalf("result publication calls = %d, want 1", store.resultCalls)
	}
	if tasks.request.OperationID != "operation-id" || tasks.request.Kind != backgroundMediaMetadataTaskKind {
		t.Fatalf("unexpected metadata finalizer request: %+v", tasks.request)
	}
}

func TestBackgroundUploadFinalizerEnqueueFailurePreventsResultPublication(t *testing.T) {
	enqueueErr := errors.New("enqueue failed")
	store := &recordingBackgroundUploadStore{}
	tasks := &recordingBackgroundTaskEnqueuer{err: enqueueErr}
	finalizing := backgroundUploadFinalizingStore{backgroundUploadWorkerStore: store, tasks: tasks}

	err := finalizing.SetBackgroundOperationResult("operation-id", UploadImportResponse{})
	if !errors.Is(err, enqueueErr) {
		t.Fatalf("publish upload result error = %v, want enqueue error", err)
	}
	if store.resultCalls != 0 {
		t.Fatalf("result publication calls = %d, want 0 after enqueue failure", store.resultCalls)
	}
}
