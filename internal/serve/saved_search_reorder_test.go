package serve

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	core "gooru.local/gooru"
	"gooru.local/types"
)

type savedSearchReorderTestLibrary struct {
	ids []string
	err error
}

func (l *savedSearchReorderTestLibrary) ListSavedSearches(context.Context, string) ([]types.SavedSearch, error) {
	return nil, nil
}

func (l *savedSearchReorderTestLibrary) CreateSavedSearch(context.Context, string, savedSearchRequest) (types.SavedSearch, error) {
	return types.SavedSearch{}, nil
}

func (l *savedSearchReorderTestLibrary) UpdateSavedSearch(context.Context, string, string, savedSearchRequest) (types.SavedSearch, error) {
	return types.SavedSearch{}, nil
}

func (l *savedSearchReorderTestLibrary) ReorderSavedSearches(_ context.Context, _ string, ids []string) error {
	l.ids = append([]string(nil), ids...)
	return l.err
}

func (l *savedSearchReorderTestLibrary) DeleteSavedSearch(context.Context, string, string) (bool, error) {
	return false, nil
}

func TestHandleSavedSearchReorderPersistsCompleteOrder(t *testing.T) {
	library := &savedSearchReorderTestLibrary{}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/saved-searches/reorder", strings.NewReader(`{"ids":["saved-c","saved-a","saved-b"]}`))
	rec := httptest.NewRecorder()

	handleSavedSearchReorder(rec, req, library, "usr_test")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	want := []string{"saved-c", "saved-a", "saved-b"}
	if len(library.ids) != len(want) {
		t.Fatalf("reorder ids = %v, want %v", library.ids, want)
	}
	for i := range want {
		if library.ids[i] != want[i] {
			t.Fatalf("reorder ids = %v, want %v", library.ids, want)
		}
	}
}

func TestHandleSavedSearchReorderRejectsInvalidPermutation(t *testing.T) {
	library := &savedSearchReorderTestLibrary{err: core.ErrInvalidSavedSearchOrder}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/saved-searches/reorder", strings.NewReader(`{"ids":["saved-a"]}`))
	rec := httptest.NewRecorder()

	handleSavedSearchReorder(rec, req, library, "usr_test")

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestHandleSavedSearchReorderRejectsMalformedRequests(t *testing.T) {
	t.Run("missing ids", func(t *testing.T) {
		library := &savedSearchReorderTestLibrary{}
		req := httptest.NewRequest(http.MethodPut, "/api/v1/saved-searches/reorder", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()

		handleSavedSearchReorder(rec, req, library, "usr_test")

		assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	})

	t.Run("wrong method", func(t *testing.T) {
		library := &savedSearchReorderTestLibrary{}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/saved-searches/reorder", nil)
		rec := httptest.NewRecorder()

		handleSavedSearchReorder(rec, req, library, "usr_test")

		assertAPIError(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
		if got := rec.Header().Get("Allow"); got != http.MethodPut {
			t.Fatalf("Allow = %q, want PUT", got)
		}
	})
}

func TestHandleSavedSearchReorderSurfacesInternalFailure(t *testing.T) {
	library := &savedSearchReorderTestLibrary{err: errors.New("database unavailable")}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/saved-searches/reorder", strings.NewReader(`{"ids":[]}`))
	rec := httptest.NewRecorder()

	handleSavedSearchReorder(rec, req, library, "usr_test")

	assertAPIError(t, rec, http.StatusInternalServerError, "internal_error")
}
