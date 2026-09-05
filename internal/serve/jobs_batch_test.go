package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
)

func TestJobCollectionFiltersRequestedIDsWithoutScanningList(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	auth := attachTestAuth(t, server)

	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		job, err := server.jobs.Submit(context.Background(), "test", true, func(ctx context.Context) (interface{}, error) {
			return "ok", nil
		})
		if err != nil {
			t.Fatalf("submit job %d: %v", i, err)
		}
		waitForStatus(t, server.jobs, job.ID, JobCompleted)
		ids = append(ids, job.ID)
	}

	query := url.Values{}
	query.Add("id", ids[2])
	query.Add("id", ids[0]+",missing")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?"+query.Encode(), nil)
	addAuthCookie(req, cfg, auth)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body JobListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 requested jobs, got %d: %+v", len(body.Items), body.Items)
	}
	if body.Items[0].ID != ids[2] || body.Items[1].ID != ids[0] {
		t.Fatalf("expected requested order %q, %q; got %q, %q", ids[2], ids[0], body.Items[0].ID, body.Items[1].ID)
	}
}

func TestJobCollectionRejectsOversizedStatusBatch(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	auth := attachTestAuth(t, server)

	query := url.Values{}
	for i := 0; i <= maxJobStatusBatch; i++ {
		query.Add("id", fmt.Sprintf("job-%d", i))
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?"+query.Encode(), nil)
	addAuthCookie(req, cfg, auth)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusBadRequest, "invalid_request")
}
