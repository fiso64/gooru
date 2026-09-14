package serve

import "testing"

type recordingBackgroundUploadTaskStateStore struct {
	*recordingBackgroundUploadStore
	getCheckpointCalls int
	setCheckpointCalls int
	setResultCalls     int
}

func (s *recordingBackgroundUploadTaskStateStore) GetBackgroundTaskCheckpoint(string, any) (bool, error) {
	s.getCheckpointCalls++
	return false, nil
}

func (s *recordingBackgroundUploadTaskStateStore) SetBackgroundTaskCheckpoint(string, any) error {
	s.setCheckpointCalls++
	return nil
}

func (s *recordingBackgroundUploadTaskStateStore) SetBackgroundTaskResult(string, any) error {
	s.setResultCalls++
	return nil
}

func TestBackgroundUploadFinalizingStorePreservesTaskStateCapability(t *testing.T) {
	narrow := newBackgroundUploadFinalizingStore(&recordingBackgroundUploadStore{}, nil)
	if _, ok := narrow.(backgroundUploadTaskStateStore); ok {
		t.Fatal("narrow upload store unexpectedly advertises task-scoped recovery")
	}

	backing := &recordingBackgroundUploadTaskStateStore{recordingBackgroundUploadStore: &recordingBackgroundUploadStore{}}
	wrapped := newBackgroundUploadFinalizingStore(backing, nil)
	taskStore, ok := wrapped.(backgroundUploadTaskStateStore)
	if !ok {
		t.Fatal("task-scoped upload recovery capability was lost by finalizing store")
	}
	if _, err := taskStore.GetBackgroundTaskCheckpoint("task-id", nil); err != nil {
		t.Fatalf("get task checkpoint: %v", err)
	}
	if err := taskStore.SetBackgroundTaskCheckpoint("task-id", struct{}{}); err != nil {
		t.Fatalf("set task checkpoint: %v", err)
	}
	if err := taskStore.SetBackgroundTaskResult("task-id", struct{}{}); err != nil {
		t.Fatalf("set task result: %v", err)
	}
	if backing.getCheckpointCalls != 1 || backing.setCheckpointCalls != 1 || backing.setResultCalls != 1 {
		t.Fatalf("task-scoped recovery calls were not delegated: get=%d set_checkpoint=%d set_result=%d", backing.getCheckpointCalls, backing.setCheckpointCalls, backing.setResultCalls)
	}
}
