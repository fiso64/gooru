package serve

import (
	"bytes"
	"io"
	"testing"

	core "gooru.local/gooru"
)

type receivingProgressTestStore struct {
	checkpoint backgroundUploadCheckpoint
	state      core.BackgroundOperationState
}

func (s *receivingProgressTestStore) SetBackgroundOperationCheckpoint(_ string, checkpoint any) error {
	s.checkpoint = checkpoint.(backgroundUploadCheckpoint)
	return nil
}

func (s *receivingProgressTestStore) GetBackgroundOperation(_ string) (core.BackgroundOperationState, bool, error) {
	return s.state, true, nil
}

func TestUploadReceivingProgressPersistsKnownLengthBytes(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 1000)
	store := &receivingProgressTestStore{state: core.BackgroundOperationState{Status: core.BackgroundWorkPending}}
	body := newUploadReceivingProgressReadCloser(io.NopCloser(bytes.NewReader(payload)), "operation", int64(len(payload)), store)
	if _, err := io.Copy(io.Discard, body); err != nil {
		t.Fatal(err)
	}
	if store.checkpoint.Phase != backgroundUploadPhaseReceiving || store.checkpoint.TransportBytesTotal != 1000 || store.checkpoint.TransportBytesReceived != 1000 {
		t.Fatalf("receiving checkpoint = %+v", store.checkpoint)
	}
}

func TestUploadReceivingProgressUnknownLengthKeepsTotalUnknown(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 128)
	store := &receivingProgressTestStore{state: core.BackgroundOperationState{Status: core.BackgroundWorkPending}}
	body := newUploadReceivingProgressReadCloser(io.NopCloser(bytes.NewReader(payload)), "operation", -1, store)
	if _, err := io.Copy(io.Discard, body); err != nil {
		t.Fatal(err)
	}
	if store.checkpoint.TransportBytesTotal != 0 || store.checkpoint.TransportBytesReceived != int64(len(payload)) {
		t.Fatalf("unknown-length checkpoint = %+v", store.checkpoint)
	}
}
