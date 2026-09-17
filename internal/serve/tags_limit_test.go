package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTagsZeroLimitReturnsCompleteSet(t *testing.T) {
	server, cleanup := newTestBrowseServer(t)
	defer cleanup()

	limited := httptest.NewRecorder()
	server.Handler().ServeHTTP(limited, authedRequest(http.MethodGet, "/api/v1/tags?counts=true&limit=1"))
	if limited.Code != http.StatusOK {
		t.Fatalf("expected limited tags 200, got %d: %s", limited.Code, limited.Body.String())
	}
	var limitedResponse TagListResponse
	if err := json.Unmarshal(limited.Body.Bytes(), &limitedResponse); err != nil {
		t.Fatalf("decode limited tags response: %v", err)
	}
	if len(limitedResponse.Tags) != 1 {
		t.Fatalf("expected one limited tag, got %+v", limitedResponse.Tags)
	}

	complete := httptest.NewRecorder()
	server.Handler().ServeHTTP(complete, authedRequest(http.MethodGet, "/api/v1/tags?counts=true&limit=0"))
	if complete.Code != http.StatusOK {
		t.Fatalf("expected complete tags 200, got %d: %s", complete.Code, complete.Body.String())
	}
	var completeResponse TagListResponse
	if err := json.Unmarshal(complete.Body.Bytes(), &completeResponse); err != nil {
		t.Fatalf("decode complete tags response: %v", err)
	}
	if len(completeResponse.Tags) <= len(limitedResponse.Tags) {
		t.Fatalf("limit=0 should return the complete tag set, limited=%d complete=%d", len(limitedResponse.Tags), len(completeResponse.Tags))
	}
	if len(completeResponse.Tags) != 3 {
		t.Fatalf("expected all three indexed tag rows, got %+v", completeResponse.Tags)
	}
}
