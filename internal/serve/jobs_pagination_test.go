package serve

import (
	"fmt"
	"testing"
	"time"
)

func TestJobManagerListPageKeepsLatestRowsAndActiveCount(t *testing.T) {
	m := NewJobManagerWithLimits(8, 1, 0, time.Hour)
	base := time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC)

	m.mu.Lock()
	for i := 0; i < 250; i++ {
		status := JobCompleted
		if i == 3 {
			status = JobRunning
		}
		id := fmt.Sprintf("job-%03d", i)
		m.jobs[id] = &Job{ID: id, Type: "test", Status: status, SubmittedAt: base.Add(time.Duration(i) * time.Second)}
	}
	m.mu.Unlock()

	first, active := m.ListPage("", Page{Limit: 20})
	if active != 1 {
		t.Fatalf("active count = %d, want 1", active)
	}
	if len(first.Items) != 20 {
		t.Fatalf("first page length = %d, want 20", len(first.Items))
	}
	if got := first.Items[0].ID; got != "job-249" {
		t.Fatalf("first item = %q, want job-249", got)
	}
	if got := first.Items[19].ID; got != "job-230" {
		t.Fatalf("last first-page item = %q, want job-230", got)
	}
	if first.NextPageToken == "" {
		t.Fatal("expected next page token")
	}

	page, err := ParsePage("20", first.NextPageToken)
	if err != nil {
		t.Fatalf("parse next token: %v", err)
	}
	second, active := m.ListPage("", page)
	if active != 1 {
		t.Fatalf("second-page active count = %d, want 1", active)
	}
	if got := second.Items[0].ID; got != "job-229" {
		t.Fatalf("second page first item = %q, want job-229", got)
	}
}

func TestJobManagerListPageFiltersBeforePaging(t *testing.T) {
	m := NewJobManagerWithLimits(8, 1, 0, time.Hour)
	base := time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC)

	m.mu.Lock()
	for i := 0; i < 30; i++ {
		status := JobCompleted
		if i%3 == 0 {
			status = JobFailed
		}
		id := fmt.Sprintf("job-%02d", i)
		m.jobs[id] = &Job{ID: id, Type: "test", Status: status, SubmittedAt: base.Add(time.Duration(i) * time.Second)}
	}
	m.mu.Unlock()

	page, _ := m.ListPage(string(JobFailed), Page{Limit: 4})
	if len(page.Items) != 4 {
		t.Fatalf("filtered page length = %d, want 4", len(page.Items))
	}
	want := []string{"job-27", "job-24", "job-21", "job-18"}
	for i, job := range page.Items {
		if job.ID != want[i] {
			t.Fatalf("item %d = %q, want %q", i, job.ID, want[i])
		}
	}
	if page.NextPageToken == "" {
		t.Fatal("expected filtered next page token")
	}
}
