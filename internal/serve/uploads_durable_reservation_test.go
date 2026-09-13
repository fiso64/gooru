package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDurableUploadReservationDefaultsSegmentCountToOne(t *testing.T) {
	store := newDurableUploadTestStore()
	server := newDurableUploadHandlerTestServer(t, t.TempDir(), store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	req.Header.Set(uploadReservationHeader, "true")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := store.createdRequest.ProgressTotal; got != 1 {
		t.Fatalf("progress total = %d, want 1", got)
	}
}

func TestDurableUploadReservationUsesExpectedSegmentCount(t *testing.T) {
	store := newDurableUploadTestStore()
	server := newDurableUploadHandlerTestServer(t, t.TempDir(), store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	req.Header.Set(uploadReservationHeader, "true")
	req.Header.Set(uploadSegmentCountHeader, "625")
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := store.createdRequest.ProgressTotal; got != 625 {
		t.Fatalf("progress total = %d, want 625", got)
	}
}

func TestDurableUploadReservationRejectsInvalidSegmentCount(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "not-a-number", "1.5"} {
		t.Run(value, func(t *testing.T) {
			store := newDurableUploadTestStore()
			server := newDurableUploadHandlerTestServer(t, t.TempDir(), store)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
			req.Header.Set(uploadReservationHeader, "true")
			req.Header[uploadSegmentCountHeader] = []string{value}
			rec := httptest.NewRecorder()

			server.Handler().ServeHTTP(rec, req)

			assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
			if store.createdRequest.Kind != "" {
				t.Fatalf("invalid segment count created operation: %+v", store.createdRequest)
			}
			if store.visibleCalls != 0 || store.checkpointCalls != 0 {
				t.Fatalf("invalid segment count published reservation: visible=%d checkpoints=%d", store.visibleCalls, store.checkpointCalls)
			}
		})
	}
}

func TestDurableUploadReservationRejectsMultipleSegmentCounts(t *testing.T) {
	store := newDurableUploadTestStore()
	server := newDurableUploadHandlerTestServer(t, t.TempDir(), store)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads", nil)
	req.Header.Set(uploadReservationHeader, "true")
	req.Header[uploadSegmentCountHeader] = []string{"1", "2"}
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	if store.createdRequest.Kind != "" {
		t.Fatalf("multiple segment counts created operation: %+v", store.createdRequest)
	}
}
