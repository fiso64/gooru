package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeBackgroundOperationHistoryClearer struct {
	fakeBackgroundOperationReader
	cleared  int64
	clearErr error
	calls    int
}

func (f *fakeBackgroundOperationHistoryClearer) ClearTerminalBackgroundOperations() (int64, error) {
	f.calls++
	if f.clearErr != nil {
		return 0, f.clearErr
	}
	return f.cleared, nil
}

func TestHandleOperationsClearsTerminalHistory(t *testing.T) {
	reader := &fakeBackgroundOperationHistoryClearer{cleared: 4}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if reader.calls != 1 {
		t.Fatalf("clear calls = %d, want 1", reader.calls)
	}
	if response.Body.String() != "{\"cleared\":4}\n" {
		t.Fatalf("body = %q", response.Body.String())
	}
}

func TestHandleOperationsReportsClearFailure(t *testing.T) {
	reader := &fakeBackgroundOperationHistoryClearer{clearErr: errors.New("boom")}
	server := &Server{backgroundOperations: reader}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleOperationsRequiresClearCapability(t *testing.T) {
	server := &Server{backgroundOperations: &fakeBackgroundOperationReader{}}
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/operations", nil)
	response := httptest.NewRecorder()

	server.handleOperations(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
}
