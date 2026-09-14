package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type maintenanceJobRunnerStub struct {
	created bool
	err     error
	calls   int
}

func (s *maintenanceJobRunnerStub) EnsureMediaMetadataSweep() (bool, error) {
	s.calls++
	return s.created, s.err
}

func TestMaintenanceJobsCatalogIsDiscoverable(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/maintenance-jobs", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var payload MaintenanceJobListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode catalog: %v", err)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("catalog items = %d, want 1: %+v", len(payload.Items), payload.Items)
	}
	job := payload.Items[0]
	if job.ID != maintenanceJobMediaMetadataSweepID || job.Name == "" || job.Description == "" {
		t.Fatalf("unexpected catalog job: %+v", job)
	}
}

func TestMaintenanceJobInvocationUsesDurableSweepRunner(t *testing.T) {
	runner := &maintenanceJobRunnerStub{created: true}
	server := &Server{maintenanceJobs: runner}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance-jobs/media-metadata-sweep", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusAccepted, response.Body.String())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	var payload MaintenanceJobRunResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode run response: %v", err)
	}
	if !payload.Created || payload.Job.ID != maintenanceJobMediaMetadataSweepID {
		t.Fatalf("unexpected run response: %+v", payload)
	}
}

func TestMaintenanceJobInvocationReturnsExplicitNotFound(t *testing.T) {
	runner := &maintenanceJobRunnerStub{created: true}
	server := &Server{maintenanceJobs: runner}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/maintenance-jobs/does-not-exist", nil)
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if payload.Error.Code != "maintenance_job_not_found" {
		t.Fatalf("error code = %q, want maintenance_job_not_found", payload.Error.Code)
	}
}
