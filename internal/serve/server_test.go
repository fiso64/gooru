package serve

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHealthIsUnauthenticated(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected health 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestJobRoutesRequireSession(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	attachTestAuth(t, server)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/missing", nil)

	server.Handler().ServeHTTP(rec, req)

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

func TestSecurityHeadersAreApplied(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := rec.Header().Get("Referrer-Policy"); got == "" {
		t.Fatal("expected Referrer-Policy header")
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'none'") {
		t.Fatalf("expected conservative CSP, got %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); strings.Contains(got, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("default CSP should not allow inline scripts, got %q", got)
	}
}

func TestAPIMethodErrorsUseJSONEnvelope(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/health", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("expected Allow GET, got %q", got)
	}
}

func TestAPIPathErrorsUseJSONEnvelope(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)

	NewServer(cfg).Handler().ServeHTTP(rec, req)

	assertAPIError(t, rec, http.StatusNotFound, "not_found")
}

func TestJobRoutesAcceptSessionCookie(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	auth := attachTestAuth(t, server)
	job, err := server.jobs.Submit(context.Background(), "test", true, func(ctx context.Context) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/jobs/"+job.ID, nil)
	addAuthCookie(req, cfg, auth)

	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestJobCollectionListsAndClearsFinishedJobs(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "gooru.db"))
	server := NewServer(cfg)
	auth := attachTestAuth(t, server)
	job, err := server.jobs.Submit(context.Background(), "test", true, func(ctx context.Context) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	waitForStatus(t, server.jobs, job.ID, JobCompleted)

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/jobs?status=completed", nil)
	addAuthCookie(listReq, cfg, auth)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	var listed JobListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode job list: %v", err)
	}
	if len(listed.Items) != 1 || listed.Items[0].ID != job.ID {
		t.Fatalf("unexpected job list: %+v", listed)
	}

	clearReq := httptest.NewRequest(http.MethodDelete, "/api/v1/jobs?status=completed", nil)
	addAuthCookie(clearReq, cfg, auth)
	addCSRF(clearReq, auth)
	clearRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(clearRec, clearReq)
	if clearRec.Code != http.StatusOK {
		t.Fatalf("expected clear 200, got %d: %s", clearRec.Code, clearRec.Body.String())
	}
	if _, ok := server.jobs.Get(job.ID); ok {
		t.Fatal("expected completed job to be cleared")
	}
}

func TestJobsRunSerially(t *testing.T) {
	mgr := NewJobManager(2, time.Hour)
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

func TestJobsRespectMaxRunning(t *testing.T) {
	mgr := NewJobManagerWithLimits(4, 2, 0, time.Hour)
	started := make(chan string, 2)
	release := make(chan struct{})
	for _, typ := range []string{"first", "second"} {
		typ := typ
		if _, err := mgr.Submit(context.Background(), typ, true, func(ctx context.Context) (interface{}, error) {
			started <- typ
			<-release
			return typ, nil
		}); err != nil {
			t.Fatalf("submit %s: %v", typ, err)
		}
	}

	seen := map[string]bool{<-started: true, <-started: true}
	close(release)
	if !seen["first"] || !seen["second"] {
		t.Fatalf("expected both jobs to run concurrently, saw %+v", seen)
	}
}

func TestJobQueueFullReturnsStableError(t *testing.T) {
	mgr := NewJobManagerWithLimits(1, 1, 0, time.Hour)
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := mgr.Submit(context.Background(), "running", true, func(ctx context.Context) (interface{}, error) {
		close(started)
		<-release
		return nil, nil
	}); err != nil {
		t.Fatalf("submit running: %v", err)
	}
	<-started
	if _, err := mgr.Submit(context.Background(), "queued", true, func(ctx context.Context) (interface{}, error) { return nil, nil }); err != nil {
		t.Fatalf("submit queued: %v", err)
	}
	if _, err := mgr.Submit(context.Background(), "full", true, func(ctx context.Context) (interface{}, error) { return nil, nil }); !errors.Is(err, ErrJobQueueFull) {
		t.Fatalf("expected ErrJobQueueFull, got %v", err)
	}
	close(release)
}

func TestOversizedAsyncJobResultIsOmittedWithoutChangingSuccess(t *testing.T) {
	mgr := NewJobManagerWithLimits(2, 1, 8, time.Hour)
	job, err := mgr.Submit(context.Background(), "large", true, func(ctx context.Context) (interface{}, error) {
		return strings.Repeat("x", 64), nil
	})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	waitForStatus(t, mgr, job.ID, JobCompleted)
	got, ok := mgr.Get(job.ID)
	if !ok {
		t.Fatal("job disappeared")
	}
	if got.Result != nil || !got.ResultOmitted || got.Error != "" {
		t.Fatalf("expected completed job with omitted oversized result, got %+v", got)
	}
}

func TestOversizedSyncJobReturnsResultButDoesNotRetainIt(t *testing.T) {
	mgr := NewJobManagerWithLimits(2, 1, 8, time.Hour)
	want := strings.Repeat("x", 64)
	job, err := mgr.Submit(context.Background(), "large-sync", false, func(ctx context.Context) (interface{}, error) {
		return want, nil
	})
	if err != nil {
		t.Fatalf("submit sync job: %v", err)
	}
	if job.Status != JobCompleted || job.Result != want {
		t.Fatalf("sync caller must receive successful result, got %+v", job)
	}
	retained, ok := mgr.Get(job.ID)
	if !ok {
		t.Fatal("job disappeared")
	}
	if retained.Status != JobCompleted || retained.Result != nil || !retained.ResultOmitted || retained.Error != "" {
		t.Fatalf("expected completed retained job with result omitted, got %+v", retained)
	}
}

func TestAsyncJobSurvivesSubmittingContextCancellation(t *testing.T) {
	mgr := NewJobManager(2, time.Hour)
	reqCtx, cancelReq := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})

	job, err := mgr.Submit(reqCtx, "async", true, func(ctx context.Context) (interface{}, error) {
		close(started)
		<-release
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("submit async job: %v", err)
	}
	<-started
	cancelReq()
	close(release)

	waitForStatus(t, mgr, job.ID, JobCompleted)
}

func TestSyncJobCancelsWithSubmittingContext(t *testing.T) {
	mgr := NewJobManager(2, time.Hour)
	reqCtx, cancelReq := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		_, err := mgr.Submit(reqCtx, "sync", false, func(ctx context.Context) (interface{}, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		done <- err
	}()

	<-started
	cancelReq()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected sync submit to return cancellation error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync cancellation")
	}
}

func TestCancelPendingJobSkipsRun(t *testing.T) {
	mgr := NewJobManager(2, time.Hour)
	block := make(chan struct{})
	firstStarted := make(chan struct{})
	ranCanceled := make(chan struct{}, 1)

	first, err := mgr.Submit(context.Background(), "first", true, func(ctx context.Context) (interface{}, error) {
		close(firstStarted)
		<-block
		return "first-result", nil
	})
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	<-firstStarted

	second, err := mgr.Submit(context.Background(), "second", true, func(ctx context.Context) (interface{}, error) {
		ranCanceled <- struct{}{}
		return "second-result", nil
	})
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if _, ok := mgr.Cancel(second.ID); !ok {
		t.Fatal("expected pending job to be cancelable")
	}

	close(block)
	waitForStatus(t, mgr, first.ID, JobCompleted)
	waitForStatus(t, mgr, second.ID, JobCanceled)
	select {
	case <-ranCanceled:
		t.Fatal("canceled pending job ran")
	default:
	}
}

func TestSyncPendingJobCanceledByRequestContextSkipsRun(t *testing.T) {
	mgr := NewJobManager(2, time.Hour)
	block := make(chan struct{})
	firstStarted := make(chan struct{})
	ranCanceled := make(chan struct{}, 1)

	first, err := mgr.Submit(context.Background(), "first", true, func(ctx context.Context) (interface{}, error) {
		close(firstStarted)
		<-block
		return "first-result", nil
	})
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	<-firstStarted

	reqCtx, cancelReq := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := mgr.Submit(reqCtx, "second", false, func(ctx context.Context) (interface{}, error) {
			ranCanceled <- struct{}{}
			return "second-result", nil
		})
		done <- err
	}()
	second := waitForJobType(t, mgr, "second")
	cancelReq()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected sync submit cancellation error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync submit to return")
	}

	close(block)
	waitForStatus(t, mgr, first.ID, JobCompleted)
	waitForStatus(t, mgr, second.ID, JobCanceled)
	select {
	case <-ranCanceled:
		t.Fatal("request-canceled pending sync job ran")
	default:
	}
}

func TestCompletedJobsExpireAfterTTL(t *testing.T) {
	mgr := NewJobManager(2, 20*time.Millisecond)
	job, err := mgr.Submit(context.Background(), "short", true, func(ctx context.Context) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("submit async job: %v", err)
	}
	waitForStatus(t, mgr, job.ID, JobCompleted)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for completed job to expire")
		default:
			if _, ok := mgr.Get(job.ID); !ok {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
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

func waitForJobType(t *testing.T, mgr *JobManager, typ string) *Job {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job type %s", typ)
		default:
			mgr.mu.RLock()
			for _, job := range mgr.jobs {
				if job.Type == typ {
					cp := cloneJob(job)
					mgr.mu.RUnlock()
					return cp
				}
			}
			mgr.mu.RUnlock()
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func saturateJobQueue(t *testing.T, server *Server) func() {
	t.Helper()
	server.jobs = NewJobManagerWithLimits(1, 1, 0, time.Hour)
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := server.jobs.Submit(context.Background(), "running", true, func(ctx context.Context) (interface{}, error) {
		close(started)
		<-release
		return nil, nil
	}); err != nil {
		t.Fatalf("submit running blocker: %v", err)
	}
	<-started
	if _, err := server.jobs.Submit(context.Background(), "queued", true, func(ctx context.Context) (interface{}, error) {
		return nil, nil
	}); err != nil {
		close(release)
		t.Fatalf("submit queued blocker: %v", err)
	}
	return func() {
		close(release)
	}
}

func assertAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("expected status %d, got %d: %s", status, rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected JSON content type, got %q", got)
	}
	var body ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid error JSON: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("expected error code %q, got %q", code, body.Error.Code)
	}
}
