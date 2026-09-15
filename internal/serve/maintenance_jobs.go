package serve

import (
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
	RunMediaMetadataSweep() (bool, error)
	MediaMetadataSweepRunning() (bool, error)
}

var maintenanceJobCatalog = []MaintenanceJobDTO{
	{
		ID:          maintenanceJobMediaMetadataSweepID,
		Name:        "Extract media metadata",
		Description: "Scan tracked files that are missing media metadata and enqueue durable extraction work.",
	},
}

func (l *GooruLibrary) RunMediaMetadataSweep() (bool, error) {
	return l.client.RunMediaMetadataSweep()
}

func (l *GooruLibrary) MediaMetadataSweepRunning() (bool, error) {
	return l.client.MediaMetadataSweepRunning()
}

func (s *Server) handleMaintenanceJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	items := append([]MaintenanceJobDTO(nil), maintenanceJobCatalog...)
	if s.maintenanceJobs != nil {
		running, err := s.maintenanceJobs.MediaMetadataSweepRunning()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to inspect maintenance job state", nil)
			return
		}
		for i := range items {
			if items[i].ID == maintenanceJobMediaMetadataSweepID {
				items[i].Running = running
			}
		}
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
	job, found := maintenanceJobByID(id)
	if !found || id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "maintenance_job_not_found", "maintenance job not found", nil)
		return
	}
	if s.maintenanceJobs == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "maintenance job service is not configured", nil)
		return
	}

	var (
		created bool
		err     error
	)
	switch id {
	case maintenanceJobMediaMetadataSweepID:
		created, err = s.maintenanceJobs.RunMediaMetadataSweep()
	default:
		writeError(w, http.StatusNotFound, "maintenance_job_not_found", "maintenance job not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to enqueue maintenance job", nil)
		return
	}
	// A successful manual invocation either created this operation or raced an
	// already-active run. In both cases the catalog entry is currently running.
	job.Running = true
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, MaintenanceJobRunResponse{Job: job, Created: created})
}

func maintenanceJobByID(id string) (MaintenanceJobDTO, bool) {
	for _, job := range maintenanceJobCatalog {
		if job.ID == id {
			return job, true
		}
	}
	return MaintenanceJobDTO{}, false
}
