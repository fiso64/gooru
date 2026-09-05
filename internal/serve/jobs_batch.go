package serve

import (
	"fmt"
	"net/url"
	"strings"
)

const maxJobStatusBatch = 64

func jobIDsFromQuery(values url.Values) ([]string, error) {
	raw := values["id"]
	if len(raw) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			id := strings.TrimSpace(part)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
			if len(ids) > maxJobStatusBatch {
				return nil, fmt.Errorf("at most %d job ids may be requested", maxJobStatusBatch)
			}
		}
	}
	return ids, nil
}

// ListIDs returns snapshots for only the requested jobs. Unlike List, this is
// O(k) in the request size and does not scan or sort every retained completed
// job, which keeps upload status reconciliation bounded for large batches.
func (m *JobManager) ListIDs(ids []string, status string) []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*Job, 0, len(ids))
	for _, id := range ids {
		job, ok := m.jobs[id]
		if !ok {
			continue
		}
		if status != "" && string(job.Status) != status {
			continue
		}
		out = append(out, cloneJob(job))
	}
	return out
}
