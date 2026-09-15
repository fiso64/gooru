package serve

import (
	"errors"
	"net/http"
	"strings"
)

const maintenanceJobMediaMetadataSweepID = "media-metadata-sweep"

type MaintenanceJobDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Running     bool   `json:"running"`
}

type MaintenanceJobListResponse struct {
	Items []MaintenanceJobDTO `json:"items"`
}

type MaintenanceJobRunResponse struct {
	Job     MaintenanceJobDTO `json:"job"`
	Created bool              `json:"created"`
}

type maintenanceJobRunner interface {
	ListMaintenanceJobs() ([]MaintenanceJobDTO, error)
	RunMaintenanceJob(string) (MaintenanceJobDTO, bool, error)
}

type maintenanceJobDefinition struct {
	job     MaintenanceJobDTO
	running func(*GooruLibrary) (bool, error)
	run     func(*GooruLibrary) (bool, error)
}

var errMaintenanceJobNotFound = errors.New("maintenance job not found")

var maintenanceJobCatalog = []maintenanceJobDefinition{
	{
		job: MaintenanceJobDTO{
			ID:          maintenanceJobMediaMetadataSweepID,
			Name:        "Extract media metadata",
			Description: "Scan tracked files that are missing media metadata and enqueue durable extraction work.",
		},
		running: func(l *GooruLibrary) (bool, error) {
			return l.client.MediaMetadataSweepRunning()
		},
		run: func(l *GooruLibrary) (bool, error) {
			return l.client.RunMediaMetadataSweep()
		},
	},
}

func (l *GooruLibrary) ListMaintenanceJobs() ([]MaintenanceJobDTO, error) {
	items := make([]MaintenanceJobDTO, 0, len(maintenanceJobCatalog))
	for _, definition := range maintenanceJobCatalog {
		job := definition.job
		running, err := definition.running(l)
		if err != nil {
			return nil, err
		}
		job.Running = running
		items = append(items, job)
	}
	return items, nil
}

func (l *GooruLibrary) RunMaintenanceJob(id string) (MaintenanceJobDTO, bool, error) {
	definition, found := maintenanceJobByID(id)
	if !found {
		return MaintenanceJobDTO{}, false, errMaintenanceJobNotFound
	}
	created, err := definition.run(l)
	if err != nil {
		return MaintenanceJobDTO{}, false, err
	}
	job := definition.job
	job.Running = true
	return job, created, nil
}

func (s *Server) handleMaintenanceJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	if s.maintenanceJobs == nil {
		items := make([]MaintenanceJobDTO, 0, len(maintenanceJobCatalog))
		for _, definition := range maintenanceJobCatalog {
			items = append(items, definition.job)
		}
		writeJSON(w, http.StatusOK, MaintenanceJobListResponse{Items: items})
		return
	}
	items, err := s.maintenanceJobs.ListMaintenanceJobs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to inspect maintenance job state", nil)
		return
	}
	writeJSON(w, http.StatusOK, MaintenanceJobListResponse{Items: items})
}

func (s *Server) handleMaintenanceJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/maintenance-jobs/")
	if _, found := maintenanceJobByID(id); !found || id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "maintenance_job_not_found", "maintenance job not found", nil)
		return
	}
	if s.maintenanceJobs == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "maintenance job service is not configured", nil)
		return
	}

	job, created, err := s.maintenanceJobs.RunMaintenanceJob(id)
	if errors.Is(err, errMaintenanceJobNotFound) {
		writeError(w, http.StatusNotFound, "maintenance_job_not_found", "maintenance job not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to enqueue maintenance job", nil)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, MaintenanceJobRunResponse{Job: job, Created: created})
}

func maintenanceJobByID(id string) (maintenanceJobDefinition, bool) {
	for _, definition := range maintenanceJobCatalog {
		if definition.job.ID == id {
			return definition, true
		}
	}
	return maintenanceJobDefinition{}, false
}
