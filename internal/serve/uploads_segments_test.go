package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"

	core "gooru.local/gooru"
)

type durableUploadSegmentTestStore struct {
	*durableUploadTestStore
	tasks map[string]core.BackgroundTaskState
}

func newDurableUploadSegmentTestStore(segmentCount int64) *durableUploadSegmentTestStore {
	base := newDurableUploadTestStore()
	base.operation.ProgressTotal = segmentCount
	base.state.ProgressTotal = segmentCount
	base.state.Visible = true
	return &durableUploadSegmentTestStore{durableUploadTestStore: base, tasks: map[string]core.BackgroundTaskState{}}
}

func (s *durableUploadSegmentTestStore) GetBackgroundTask(taskID string) (core.BackgroundTaskState, bool, error) {
	task, found := s.tasks[taskID]
	return task, found, nil
}

func (s *durableUploadSegmentTestStore) AttachBackgroundTaskToOperation(operationID, taskID string, _ any, request core.BackgroundTaskRequest) (core.BackgroundTask, bool, error) {
	if existing, found := s.tasks[taskID]; found {
		return existing.BackgroundTask, false, nil
	}
	task := core.BackgroundTask{ID: taskID, OperationID: operationID, Kind: request.Kind, SubjectKind: request.SubjectKind, SubjectID: request.SubjectID, InputKey: request.InputKey}
	s.tasks[taskID] = core.BackgroundTaskState{BackgroundTask: task, Status: core.BackgroundWorkPending}
	return task, true, nil
}

func TestDurableUploadSegmentIndexValidation(t *testing.T) {
	for _, test := range []struct {
		name    string
		values  []string
		index   int64
		present bool
		wantErr bool
	}{
		{name: "absent"},
		{name: "zero", values: []string{"0"}, index: 0, present: true},
		{name: "positive", values: []string{"12"}, index: 12, present: true},
		{name: "negative", values: []string{"-1"}, present: true, wantErr: true},
		{name: "invalid", values: []string{"nope"}, present: true, wantErr: true},
		{name: "duplicate", values: []string{"0", "1"}, present: true, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
			for _, value := range test.values {
				req.Header.Add(uploadSegmentIndexHeader, value)
			}
			index, present, err := durableUploadSegmentIndex(req)
			if (err != nil) != test.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, test.wantErr)
			}
			if present != test.present || (!test.wantErr && index != test.index) {
				t.Fatalf("index/present = %d/%v, want %d/%v", index, present, test.index, test.present)
			}
		})
	}
}

func TestAdmitDurableUploadSegmentUsesStableTaskIdentityWithoutProducerClaim(t *testing.T) {
	store := newDurableUploadSegmentTestStore(3)
	server := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	req.Header.Set("Prefer", "respond-async")
	req.Header.Set(uploadOperationHeader, store.operation.ID)
	req.Header.Set(uploadSegmentIndexHeader, "1")

	admission, err := server.admitDurableUpload(req, store)
	if err != nil {
		t.Fatalf("admit first segment: %v", err)
	}
	if !admission.segmented || admission.segmentIndex != 1 || admission.alreadyAttached || admission.created {
		t.Fatalf("unexpected first admission: %+v", admission)
	}
	if store.producerClaimed {
		t.Fatal("segmented admission consumed legacy producer claim")
	}
	wantTaskID := durableUploadSegmentTaskID(store.operation.ID, 1)
	if admission.taskID != wantTaskID {
		t.Fatalf("task id = %q, want %q", admission.taskID, wantTaskID)
	}

	store.tasks[wantTaskID] = core.BackgroundTaskState{BackgroundTask: core.BackgroundTask{
		ID: wantTaskID, OperationID: store.operation.ID, Kind: backgroundUploadTaskKind, SubjectKind: "operation", SubjectID: store.operation.ID,
	}, Status: core.BackgroundWorkCompleted}
	store.state.Status = core.BackgroundWorkCompleted

	retry, err := server.admitDurableUpload(req, store)
	if err != nil {
		t.Fatalf("admit completed retry: %v", err)
	}
	if !retry.alreadyAttached || retry.taskID != wantTaskID {
		t.Fatalf("unexpected retry admission: %+v", retry)
	}
}

func TestAdmitDurableUploadSegmentRejectsOutOfRangeAndSyncTransport(t *testing.T) {
	store := newDurableUploadSegmentTestStore(2)
	server := &Server{}

	outOfRange := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	outOfRange.Header.Set("Prefer", "respond-async")
	outOfRange.Header.Set(uploadOperationHeader, store.operation.ID)
	outOfRange.Header.Set(uploadSegmentIndexHeader, "2")
	if _, err := server.admitDurableUpload(outOfRange, store); err == nil {
		t.Fatal("out-of-range segment unexpectedly admitted")
	}

	syncRequest := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	syncRequest.Header.Set(uploadOperationHeader, store.operation.ID)
	syncRequest.Header.Set(uploadSegmentIndexHeader, "0")
	if _, err := server.admitDurableUpload(syncRequest, store); err == nil {
		t.Fatal("segmented sync request unexpectedly admitted")
	}
}

func TestDurableUploadSegmentsPersistIndependentTags(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadSegmentTestStore(2)
	server := newUploadTestServer(t, targetDir, true, &recordingUploadLibrary{})
	server.backgroundOperations = store

	segments := []struct {
		indexHeader string
		index       int64
		name        string
		tags        []string
	}{
		{indexHeader: "0", index: 0, name: "first.jpg", tags: []string{"artist:first", "rating:safe"}},
		{indexHeader: "1", index: 1, name: "second.jpg", tags: []string{"artist:second", "series:two"}},
	}

	for _, segment := range segments {
		req := uploadRequest(t, map[string]string{segment.name: "hello"}, segment.tags)
		req.Header.Set("Prefer", "respond-async")
		req.Header.Set(uploadOperationHeader, store.operation.ID)
		req.Header.Set(uploadSegmentIndexHeader, segment.indexHeader)
		rec := httptest.NewRecorder()

		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("segment %d status = %d: %s", segment.index, rec.Code, rec.Body.String())
		}
		taskID := durableUploadSegmentTaskID(store.operation.ID, segment.index)
		task, found := store.tasks[taskID]
		if !found {
			t.Fatalf("segment %d task %q was not attached", segment.index, taskID)
		}
		files, tags, err := decodeBackgroundUploadTask(task.BackgroundTask)
		if err != nil {
			t.Fatalf("decode segment %d task: %v", segment.index, err)
		}
		if len(files) != 1 || files[0].name != segment.name {
			t.Fatalf("segment %d files = %#v, want %q", segment.index, files, segment.name)
		}
		if len(tags) != len(segment.tags) {
			t.Fatalf("segment %d tags = %#v, want %#v", segment.index, tags, segment.tags)
		}
		for i := range tags {
			if tags[i] != segment.tags[i] {
				t.Fatalf("segment %d tags = %#v, want %#v", segment.index, tags, segment.tags)
			}
		}
	}

	if len(store.tasks) != len(segments) {
		t.Fatalf("attached tasks = %d, want %d", len(store.tasks), len(segments))
	}
	first := store.tasks[durableUploadSegmentTaskID(store.operation.ID, 0)].InputKey
	second := store.tasks[durableUploadSegmentTaskID(store.operation.ID, 1)].InputKey
	if first == second {
		t.Fatal("segmented uploads unexpectedly shared one persisted task payload")
	}
}
