package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthIsUnauthenticated(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Token = "secret"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestJobRoutesRequireBearerToken(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	cfg.Auth.Token = "secret"
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/missing", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error JSON: %v", err)
	}
	if body.Error.Code != "unauthorized" {
		t.Fatalf("unexpected error code %q", body.Error.Code)
	}
}

func TestJobsRunSerially(t *testing.T) {
	mgr := NewJobManager(2)
	order := make(chan string, 2)
	block := make(chan struct{})
	first, err := mgr.Submit(context.Background(), "first", true, func(ctx context.Context) (interface{}, error) {
		order <- "first-start"
		<-block
		return "first-result", nil
	})
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	second, err := mgr.Submit(context.Background(), "second", true, func(ctx context.Context) (interface{}, error) {
		order <- "second-start"
		return "second-result", nil
	})
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if got := <-order; got != "first-start" {
		t.Fatalf("first job should start first, got %q", got)
	}
	select {
	case got := <-order:
		t.Fatalf("second job started before first completed: %q", got)
	default:
	}
	close(block)
	if got := <-order; got != "second-start" {
		t.Fatalf("second job did not start after first completed: %q", got)
	}
	waitForStatus(t, mgr, first.ID, JobCompleted)
	waitForStatus(t, mgr, second.ID, JobCompleted)
}

func TestPaginationTokensRoundTrip(t *testing.T) {
	page, err := ParsePage("25", "")
	if err != nil {
		t.Fatalf("ParsePage: %v", err)
	}
	token := NextPageToken(page.Offset, page.Limit, page.Limit)
	next, err := ParsePage("25", token)
	if err != nil {
		t.Fatalf("ParsePage token: %v", err)
	}
	if next.Offset != 25 || next.Limit != 25 {
		t.Fatalf("unexpected next page: %+v", next)
	}
}

func TestPreferAsync(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files/tags", nil)
	req.Header.Set("Prefer", "wait=10, respond-async")
	if !PreferAsync(req) {
		t.Fatal("expected PreferAsync to detect respond-async")
	}
}

func waitForStatus(t *testing.T, mgr *JobManager, id string, status JobStatus) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job %s status %s", id, status)
		default:
			job, ok := mgr.Get(id)
			if ok && job.Status == status {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
