package serve

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestJobLifecycleLogging(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	manager := NewJobManagerWithLimits(8, 1, 0, time.Hour)

	completed, err := manager.Submit(context.Background(), "thumbnail", true, func(context.Context) (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("submit completed job: %v", err)
	}
	waitForLoggedJobStatus(t, manager, completed.ID, JobCompleted)

	failed, err := manager.Submit(context.Background(), "import", true, func(context.Context) (interface{}, error) {
		return nil, errors.New("private-filename.jpg failed analysis")
	})
	if err != nil {
		t.Fatalf("submit failed job: %v", err)
	}
	waitForLoggedJobStatus(t, manager, failed.ID, JobFailed)

	cancelStarted := make(chan struct{})
	canceled, err := manager.Submit(context.Background(), "export", true, func(ctx context.Context) (interface{}, error) {
		close(cancelStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatalf("submit canceled job: %v", err)
	}
	select {
	case <-cancelStarted:
	case <-time.After(time.Second):
		t.Fatal("canceled job did not start")
	}
	if _, ok := manager.Cancel(canceled.ID); !ok {
		t.Fatal("cancel job: job not found")
	}
	waitForLoggedJobStatus(t, manager, canceled.ID, JobCanceled)
	waitForLogContains(t, &output, "job_type=export status=canceled")

	logs := output.String()
	for _, want := range []string{
		"msg=\"job started\"",
		"job_type=thumbnail",
		"status=completed",
		"job_type=import",
		"status=failed",
		"level=WARN msg=\"job finished\"",
		"job_type=export",
		"status=canceled",
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("job lifecycle log missing %q: %s", want, logs)
		}
	}
	if strings.Contains(logs, "private-filename.jpg") || strings.Contains(logs, "failed analysis") {
		t.Fatalf("job lifecycle log leaked job error details: %s", logs)
	}
}

func waitForLoggedJobStatus(t *testing.T, manager *JobManager, id string, want JobStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		job, ok := manager.Get(id)
		if ok && job.Status == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	job, ok := manager.Get(id)
	if !ok {
		t.Fatalf("job %s disappeared before reaching %s", id, want)
	}
	t.Fatalf("job %s status = %s, want %s", id, job.Status, want)
}

func waitForLogContains(t *testing.T, output *bytes.Buffer, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("log never contained %q: %s", want, output.String())
}
