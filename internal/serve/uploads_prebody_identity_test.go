package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	core "gooru.local/gooru"
)

type reservationCancellationStore struct {
	*durableUploadTestStore
	attachAttempts int
}

func (s *reservationCancellationStore) AttachBackgroundTaskAndRevealOperation(operationID string, checkpoint any, request core.BackgroundTaskRequest) (core.BackgroundTask, error) {
	s.attachAttempts++
	if s.state.Status != core.BackgroundWorkPending {
		return core.BackgroundTask{}, errors.New("background operation is not pending")
	}
	return s.durableUploadTestStore.AttachBackgroundTaskAndRevealOperation(operationID, checkpoint, request)
}

func TestReservedDurableUploadCancellationPreventsAttachmentWithoutRequestContextCancellation(t *testing.T) {
	targetDir := t.TempDir()
	baseStore := newDurableUploadTestStore()
	store := &reservationCancellationStore{durableUploadTestStore: baseStore}
	server := newDurableUploadHandlerTestServer(t, targetDir, baseStore)
	server.backgroundOperations = store

	reserveReq := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	reserveReq.Header.Set(uploadReservationHeader, "true")
	reserveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(reserveRec, reserveReq)
	if reserveRec.Code != http.StatusCreated {
		t.Fatalf("reservation status = %d: %s", reserveRec.Code, reserveRec.Body.String())
	}
	if !store.state.Visible || store.state.Status != core.BackgroundWorkPending {
		t.Fatalf("reservation state = %+v, want visible pending", store.state)
	}

	uploadReq := uploadRequest(t, map[string]string{"first.bin": "first", "second.bin": "second"}, nil)
	uploadReq.Header.Set("Prefer", "respond-async")
	uploadReq.Header.Set(uploadOperationHeader, store.operation.ID)
	body := &gatedUploadBody{ReadCloser: uploadReq.Body, entered: make(chan struct{}), release: make(chan struct{})}
	uploadReq.Body = body
	uploadRec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		server.Handler().ServeHTTP(uploadRec, uploadReq)
		close(done)
	}()

	<-body.entered
	cancelReq := httptest.NewRequest(http.MethodDelete, "/api/v1/operations/"+store.operation.ID, nil)
	cancelRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusAccepted {
		t.Fatalf("cancel status = %d: %s", cancelRec.Code, cancelRec.Body.String())
	}
	if store.state.Status != core.BackgroundWorkCanceled {
		t.Fatalf("operation status = %q, want canceled", store.state.Status)
	}

	close(body.release)
	<-done
	assertAPIError(t, uploadRec, http.StatusRequestTimeout, "request_canceled")
	if store.attachAttempts != 1 {
		t.Fatalf("attach attempts = %d, want 1 rejected attempt after transport completed", store.attachAttempts)
	}
	if store.attachCalls != 0 {
		t.Fatalf("successful attach calls = %d, want 0", store.attachCalls)
	}
}

func TestDurableUploadRejectsCanceledReservationBeforeReadingBody(t *testing.T) {
	targetDir := t.TempDir()
	store := newDurableUploadTestStore()
	store.state.Visible = true
	store.state.Status = core.BackgroundWorkCanceled
	server := newDurableUploadHandlerTestServer(t, targetDir, store)
	body := &uploadReadTracker{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=unused")
	req.Header.Set(uploadOperationHeader, store.operation.ID)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusConflict, "invalid_upload_reservation")
	if body.read {
		t.Fatal("request body was read for an already-canceled reservation")
	}
}
