package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCanceled  JobStatus = "canceled"
)

type Job struct {
	ID          string      `json:"id"`
	Type        string      `json:"type"`
	Status      JobStatus   `json:"status"`
	Progress    float64     `json:"progress,omitempty"`
	SubmittedAt time.Time   `json:"submitted_at"`
	StartedAt   *time.Time  `json:"started_at,omitempty"`
	FinishedAt  *time.Time  `json:"finished_at,omitempty"`
	Result      interface{} `json:"result,omitempty"`
	Error       string      `json:"error,omitempty"`

	cancel context.CancelFunc
	done   chan struct{}
}

type JobFunc func(context.Context) (interface{}, error)

type JobManager struct {
	mu           sync.RWMutex
	jobs         map[string]*Job
	queue        chan queuedJob
	completedTTL time.Duration
}

type queuedJob struct {
	job *Job
	ctx context.Context
	run JobFunc
}

func NewJobManager(buffer int, completedTTL time.Duration) *JobManager {
	if buffer <= 0 {
		buffer = 64
	}
	if completedTTL <= 0 {
		completedTTL = time.Hour
	}
	m := &JobManager{
		jobs:         make(map[string]*Job),
		queue:        make(chan queuedJob, buffer),
		completedTTL: completedTTL,
	}
	go m.worker()
	return m
}

func (m *JobManager) Submit(ctx context.Context, typ string, async bool, run JobFunc) (*Job, error) {
	if typ == "" {
		return nil, errors.New("job type is required")
	}
	if run == nil {
		return nil, errors.New("job function is required")
	}
	jobCtx, cancel := context.WithCancel(context.Background())
	job := &Job{
		ID:          newJobID(),
		Type:        typ,
		Status:      JobPending,
		SubmittedAt: time.Now().UTC(),
		cancel:      cancel,
		done:        make(chan struct{}),
	}
	m.mu.Lock()
	m.jobs[job.ID] = job
	m.mu.Unlock()

	select {
	case m.queue <- queuedJob{job: job, ctx: jobCtx, run: run}:
	case <-ctx.Done():
		cancel()
		m.mu.Lock()
		delete(m.jobs, job.ID)
		m.mu.Unlock()
		return nil, ctx.Err()
	}

	if async {
		return m.clone(job.ID), nil
	}

	select {
	case <-job.done:
		snapshot := m.clone(job.ID)
		if snapshot.Status == JobFailed {
			return snapshot, errors.New(snapshot.Error)
		}
		if snapshot.Status == JobCanceled {
			return snapshot, context.Canceled
		}
		return snapshot, nil
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	}
}

func newJobID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("job-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func (m *JobManager) Get(id string) (*Job, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[id]
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (m *JobManager) Cancel(id string) (*Job, bool) {
	m.mu.RLock()
	job, ok := m.jobs[id]
	m.mu.RUnlock()
	if !ok {
		return nil, false
	}
	job.cancel()
	return m.clone(job.ID), true
}

func (m *JobManager) clone(id string) *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneJob(m.jobs[id])
}

func (m *JobManager) worker() {
	for item := range m.queue {
		m.run(item)
	}
}

func (m *JobManager) run(item queuedJob) {
	started := time.Now().UTC()
	m.mu.Lock()
	item.job.Status = JobRunning
	item.job.StartedAt = &started
	m.mu.Unlock()

	result, err := item.run(item.ctx)

	finished := time.Now().UTC()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			item.job.Status = JobCanceled
			item.job.Error = "job canceled"
		} else {
			item.job.Status = JobFailed
			item.job.Error = err.Error()
		}
	} else {
		item.job.Status = JobCompleted
		item.job.Result = result
		item.job.Progress = 1
	}
	item.job.FinishedAt = &finished
	close(item.job.done)
	go m.expireCompleted(item.job.ID, finished, m.completedTTL)
}

func (m *JobManager) expireCompleted(id string, finishedAt time.Time, ttl time.Duration) {
	timer := time.NewTimer(ttl)
	defer timer.Stop()
	<-timer.C

	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok || job.FinishedAt == nil || !job.FinishedAt.Equal(finishedAt) {
		return
	}
	switch job.Status {
	case JobCompleted, JobFailed, JobCanceled:
		delete(m.jobs, id)
	}
}

func cloneJob(job *Job) *Job {
	if job == nil {
		return nil
	}
	cp := *job
	cp.cancel = nil
	cp.done = nil
	return &cp
}

func (j JobStatus) Valid() bool {
	switch j {
	case JobPending, JobRunning, JobCompleted, JobFailed, JobCanceled:
		return true
	default:
		return false
	}
}

func (j *Job) String() string {
	if j == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s:%s", j.ID, j.Status)
}
