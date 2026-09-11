package background

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"gooru.local/internal/database"
)

type observabilityTaskStore struct {
	retrying bool
}

func (*observabilityTaskStore) RecoverExpiredBackgroundTaskLeases(time.Time) (int, error) {
	return 0, nil
}

func (*observabilityTaskStore) ClaimNextBackgroundTask(string, string, time.Time, time.Duration) (database.BackgroundTask, bool, error) {
	return database.BackgroundTask{}, false, nil
}

func (*observabilityTaskStore) RenewBackgroundTaskLease(string, string, time.Time, time.Duration) (time.Time, error) {
	return time.Time{}, nil
}

func (*observabilityTaskStore) CompleteBackgroundTask(string, string, time.Time) error {
	return nil
}

func (s *observabilityTaskStore) FailBackgroundTask(string, string, time.Time, time.Time, string, string) (bool, error) {
	return s.retrying, nil
}

func TestRunnerLogsTerminalFailureWithoutHandlerErrorText(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	const sensitiveError = "decode /home/alice/private/secret-photo.jpg: broken image"
	store := &observabilityTaskStore{}
	runner, err := NewRunner(RunnerConfig{
		Store:         store,
		ResourceClass: "media",
		WorkerID:      "worker-media",
		Handlers: map[string]Handler{
			"media.thumbnail": func(context.Context, database.BackgroundTask) error {
				return errors.New(sensitiveError)
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	task := database.BackgroundTask{
		ID:          "task-observe",
		OperationID: "operation-observe",
		Kind:        "media.thumbnail",
	}
	if err := runner.runClaimed(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	logged := output.String()
	for _, want := range []string{
		"background task failed permanently",
		"task_id=task-observe",
		"operation_id=operation-observe",
		"kind=media.thumbnail",
		"resource_class=media",
		"error_code=handler_failed",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log %q does not contain %q", logged, want)
		}
	}
	if strings.Contains(logged, sensitiveError) || strings.Contains(logged, "secret-photo.jpg") {
		t.Fatalf("log leaked handler error text: %q", logged)
	}
}

func TestRunnerLogsRetryAtDebugWithoutHandlerErrorText(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previous)

	const sensitiveError = "open /srv/protected/private-name.png: permission denied"
	store := &observabilityTaskStore{retrying: true}
	runner, err := NewRunner(RunnerConfig{
		Store:         store,
		ResourceClass: "media",
		WorkerID:      "worker-media",
		Handlers: map[string]Handler{
			"media.thumbnail": func(context.Context, database.BackgroundTask) error {
				return errors.New(sensitiveError)
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.runClaimed(context.Background(), database.BackgroundTask{ID: "task-retry", Kind: "media.thumbnail"}); err != nil {
		t.Fatal(err)
	}

	logged := output.String()
	if !strings.Contains(logged, "background task failed; retry scheduled") || !strings.Contains(logged, "task_id=task-retry") {
		t.Fatalf("retry log missing expected metadata: %q", logged)
	}
	if strings.Contains(logged, sensitiveError) || strings.Contains(logged, "private-name.png") {
		t.Fatalf("retry log leaked handler error text: %q", logged)
	}
}
