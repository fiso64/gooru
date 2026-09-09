package serve

import (
	"container/heap"
	"sort"
)

type jobPageHeap []*Job

func (h jobPageHeap) Len() int { return len(h) }
func (h jobPageHeap) Less(i, j int) bool {
	if h[i].SubmittedAt.Equal(h[j].SubmittedAt) {
		return h[i].ID > h[j].ID
	}
	return h[i].SubmittedAt.Before(h[j].SubmittedAt)
}
func (h jobPageHeap) Swap(i, j int)   { h[i], h[j] = h[j], h[i] }
func (h *jobPageHeap) Push(value any) { *h = append(*h, value.(*Job)) }
func (h *jobPageHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	*h = old[:len(old)-1]
	return last
}

// ListPage returns only the requested newest retained jobs. It keeps at most
// offset+limit+1 lightweight pointers while scanning, then clones only the
// returned rows. List rows intentionally omit Result: callers that need a job's
// potentially large result payload must fetch that job by ID instead. The
// common history poll therefore cannot retransmit retained upload/import result
// blobs that the jobs list UI never consumes.
//
// activeCount is computed in the same pass so callers can keep activity badges
// and polling exact even when an older active job is outside the visible page.
func (m *JobManager) ListPage(status string, page Page) (result PageResult[*Job], activeCount int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keep := page.Offset + page.Limit + 1
	if keep < 1 {
		keep = 1
	}
	selected := make(jobPageHeap, 0, keep)
	for _, job := range m.jobs {
		if job.Status == JobPending || job.Status == JobRunning {
			activeCount++
		}
		if status != "" && string(job.Status) != status {
			continue
		}
		if len(selected) < keep {
			heap.Push(&selected, job)
			continue
		}
		oldest := selected[0]
		if job.SubmittedAt.After(oldest.SubmittedAt) || (job.SubmittedAt.Equal(oldest.SubmittedAt) && job.ID < oldest.ID) {
			heap.Pop(&selected)
			heap.Push(&selected, job)
		}
	}

	sort.Slice(selected, func(i, j int) bool {
		if selected[i].SubmittedAt.Equal(selected[j].SubmittedAt) {
			return selected[i].ID < selected[j].ID
		}
		return selected[i].SubmittedAt.After(selected[j].SubmittedAt)
	})
	if page.Offset >= len(selected) {
		return PageResult[*Job]{Items: []*Job{}}, activeCount
	}
	end := page.Offset + page.Limit
	hasNext := end < len(selected)
	if end > len(selected) {
		end = len(selected)
	}
	items := make([]*Job, 0, end-page.Offset)
	for _, job := range selected[page.Offset:end] {
		items = append(items, cloneJobListItem(job))
	}
	result.Items = items
	if hasNext {
		result.NextPageToken = PageOffsetToken(page.Offset + len(items))
	}
	return result, activeCount
}

func cloneJobListItem(job *Job) *Job {
	item := cloneJob(job)
	if item != nil {
		item.Result = nil
	}
	return item
}

func (m *JobManager) ActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, job := range m.jobs {
		if job.Status == JobPending || job.Status == JobRunning {
			count++
		}
	}
	return count
}
