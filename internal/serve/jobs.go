package serve

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	ErrJobQueueFull      = errors.New("job queue is full")
	ErrJobResultTooLarge = errors.New("job result is too large to retain")
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
	maxQueued    int
	queued       int
	completedTTL time.Duration
	maxResult    int64
}

type queuedJob struct {
	job     *Job
	ctx     context.Context
	run     JobFunc
	cleanup func()
}

type JobReservation struct {
	manager *JobManager
	job     *Job
	ctx     context.Context
	cancel  context.CancelFunc
	used    bool
}

func NewJobManager(buffer int, completedTTL time.Duration) *JobManager {
	return NewJobManagerWithLimits(buffer, 1, 0, completedTTL)
}

func NewJobManagerWithLimits(maxQueued int, maxRunning int, maxResultBytes int64, completedTTL time.Duration) *JobManager {
	if maxQueued <= 0 {
		maxQueued = 64
	}
	if maxRunning <= 0 {
		maxRunning = 1
	}
	if completedTTL <= 0 {
		completedTTL = time.Hour
	}
	m := &JobManager{
		jobs:         make(map[string]*Job),
		queue:        make(chan queuedJob, maxQueued),
		maxQueued:    maxQueued,
		completedTTL: completedTTL,
		maxResult:    maxResultBytes,
	}
	for i := 0; i < maxRunning; i++ {
		go m.worker()
	}
	return m
}

func (m *JobManager) Submit(ctx context.Context, typ string, async bool, run JobFunc) (*Job, error) {
	return m.SubmitWithCleanup(ctx, typ, async, run, nil)
}

func (m *JobManager) SubmitWithCleanup(ctx context.Context, typ string, async bool, run JobFunc, cleanup func()) (*Job, error) {
	if typ == "" {
		return nil, errors.New("job type is required")
	}
	if run == nil {
		return nil, errors.New("job function is required")
	}
	reservation, err := m.Reserve(ctx, typ)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}
	return reservation.Submit(ctx, async, run, cleanup)
}

func (m *JobManager) Reserve(ctx context.Context, typ string) (*JobReservation, error) {
	if typ == "" {
		return nil, errors.New("job type is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
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
	if m.queued >= m.maxQueued {
		m.mu.Unlock()
		cancel()
		return nil, ErrJobQueueFull
	}
	m.queued++
	m.jobs[job.ID] = job
	m.mu.Unlock()
	return &JobReservation{manager: m, job: job, ctx: jobCtx, cancel: cancel}, nil
}

func (r *JobReservation) Submit(ctx context.Context, async bool, run JobFunc, cleanup func()) (*Job, error) {
	if r == nil || r.manager == nil || r.job == nil {
		return nil, errors.New("job reservation is required")
	}
	if run == nil {
		return nil, errors.New("job function is required")
	}
	if r.used {
		return nil, errors.New("job reservation already used")
	}
	r.used = true
	if err := ctx.Err(); err != nil {
		r.release()
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}

	select {
	case r.manager.queue <- queuedJob{job: r.job, ctx: r.ctx, run: run, cleanup: cleanup}:
	default:
		r.release()
		if cleanup != nil {
			cleanup()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrJobQueueFull
	}

	if async {
		return r.manager.clone(r.job.ID), nil
	}

	select {
	case <-r.job.done:
		snapshot := r.manager.clone(r.job.ID)
		if snapshot.Status == JobFailed {
			return snapshot, errors.New(snapshot.Error)
		}
		if snapshot.Status == JobCanceled {
			return snapshot, context.Canceled
		}
		return snapshot, nil
	case <-ctx.Done():
		r.manager.cancelJob(r.job)
		return nil, ctx.Err()
	}
}

func (r *JobReservation) Release() {
	if r == nil || r.used {
		return
	}
	r.used = true
	r.release()
}

func (r *JobReservation) release() {
	r.cancel()
	r.manager.mu.Lock()
	delete(r.manager.jobs, r.job.ID)
	if r.manager.queued > 0 {
		r.manager.queued--
	}
	r.manager.mu.Unlock()
}

func (m *JobManager) releaseQueuedSlot() {
	m.mu.Lock()
	if m.queued > 0 {
		m.queued--
	}
	m.mu.Unlock()
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
	m.cancelJob(job)
	return m.clone(job.ID), true
}

func (m *JobManager) List(status string) []*Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Job, 0, len(m.jobs))
	for _, job := range m.jobs {
		if status != "" && string(job.Status) != status {
			continue
		}
		out = append(out, cloneJob(job))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SubmittedAt.Equal(out[j].SubmittedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].SubmittedAt.After(out[j].SubmittedAt)
	})
	return out
}

func (m *JobManager) Clear(status string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	removed := 0
	for id, job := range m.jobs {
		if job.Status == JobPending || job.Status == JobRunning {
			continue
		}
		if status != "" && string(job.Status) != status {
			continue
		}
		delete(m.jobs, id)
		removed++
	}
	return removed
}

func (m *JobManager) cancelJob(job *Job) {
	if job == nil {
		return
	}

	finished := time.Now().UTC()
	var cancel context.CancelFunc
	var expire bool
	m.mu.Lock()
	switch job.Status {
	case JobPending:
		job.Status = JobCanceled
		job.Error = "job canceled"
		job.FinishedAt = &finished
		close(job.done)
		cancel = job.cancel
		expire = true
	case JobRunning:
		cancel = job.cancel
	}
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if expire {
		go m.expireCompleted(job.ID, finished, m.completedTTL)
	}
}

func (m *JobManager) clone(id string) *Job {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneJob(m.jobs[id])
}

func (m *JobManager) worker() {
	for item := range m.queue {
		m.releaseQueuedSlot()
		m.run(item)
	}
}

func (m *JobManager) run(item queuedJob) {
	started := time.Now().UTC()
	m.mu.Lock()
	if item.job.Status == JobCanceled {
		m.mu.Unlock()
		if item.cleanup != nil {
			item.cleanup()
		}
		return
	}
	item.job.Status = JobRunning
	item.job.StartedAt = &started
	m.mu.Unlock()

	result, err := item.run(item.ctx)

	finished := time.Now().UTC()
	resultTooLarge := false
	if err == nil && m.maxResult > 0 {
		resultTooLarge = approximateResultBytes(result) > m.maxResult
	}
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
		if resultTooLarge {
			item.job.Status = JobFailed
			item.job.Error = ErrJobResultTooLarge.Error()
		} else {
			item.job.Result = result
			item.job.Progress = 1
		}
	}
	item.job.FinishedAt = &finished
	close(item.job.done)
	go m.expireCompleted(item.job.ID, finished, m.completedTTL)
}

func approximateResultBytes(value interface{}) int64 {
	if value == nil {
		return 0
	}
	switch v := value.(type) {
	case string:
		return int64(len(v))
	case []byte:
		return int64(len(v))
	}
	data, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return int64(len(data))
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
