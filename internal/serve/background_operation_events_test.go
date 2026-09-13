package serve

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "gooru.local/gooru"
)

type operationEventTestStore struct {
	changes chan struct{}
}

func (s *operationEventTestStore) GetBackgroundOperation(string) (core.BackgroundOperationState, bool, error) {
	return core.BackgroundOperationState{}, false, nil
}

func (s *operationEventTestStore) ListBackgroundOperations(core.BackgroundOperationListOptions) ([]core.BackgroundOperationState, error) {
	return nil, nil
}

func (s *operationEventTestStore) CancelBackgroundOperation(string) (bool, error) {
	return false, nil
}

func (s *operationEventTestStore) GetBackgroundOperationResult(string, any) (bool, error) {
	return false, nil
}

func (s *operationEventTestStore) SubscribeBackgroundOperationChanges() (<-chan struct{}, func()) {
	return s.changes, func() {}
}

func TestOperationEventsStreamPayloadFreeChangeSignals(t *testing.T) {
	store := &operationEventTestStore{changes: make(chan struct{}, 1)}
	server := &Server{backgroundOperations: store}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()

	response, err := httpServer.Client().Get(httpServer.URL + "/api/v1/operations/events")
	if err != nil {
		t.Fatalf("open operation event stream: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q, want no-cache", got)
	}

	reader := bufio.NewReader(response.Body)
	assertOperationEvent(t, reader)
	store.changes <- struct{}{}
	assertOperationEvent(t, reader)
}

func TestOperationEventsRejectsNonGET(t *testing.T) {
	store := &operationEventTestStore{changes: make(chan struct{}, 1)}
	server := &Server{backgroundOperations: store}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/operations/events", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if got := response.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow = %q, want GET", got)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
}

func assertOperationEvent(t *testing.T, reader *bufio.Reader) {
	t.Helper()
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read operation event: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			break
		}
		lines = append(lines, line)
	}
	if len(lines) != 2 || lines[0] != "event: operations" || lines[1] != "data:" {
		t.Fatalf("operation event = %#v, want payload-free operations signal", lines)
	}
}
