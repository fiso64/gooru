package gooru

import (
	"path/filepath"
	"reflect"
	"testing"

	"gooru.local/types"
)

func TestFileRegistrationBackgroundTasksDeduplicatesContentHashesPerTransaction(t *testing.T) {
	client := &Client{}
	var got FileRegistrationEvent
	client.SetFileRegistrationHooks(func(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
		got = event
		return []BackgroundTaskRequest{{Kind: "test.registration"}}, nil
	})

	tasks, err := client.fileRegistrationBackgroundTasks([]string{"b", "a", "b", "", "a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Kind != "test.registration" {
		t.Fatalf("tasks = %#v, want one test.registration task", tasks)
	}
	if !reflect.DeepEqual(got.ContentHashes, []string{"b", "a"}) {
		t.Fatalf("content hashes = %#v, want [b a]", got.ContentHashes)
	}
}

func TestFileRegistrationBackgroundTasksSkipsEmptyEvents(t *testing.T) {
	client := &Client{}
	called := false
	client.SetFileRegistrationHooks(func(event FileRegistrationEvent) ([]BackgroundTaskRequest, error) {
		called = true
		return nil, nil
	})

	tasks, err := client.fileRegistrationBackgroundTasks([]string{"", ""})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("registration hook called for empty content identities")
	}
	if len(tasks) != 0 {
		t.Fatalf("tasks = %#v, want none", tasks)
	}
}

func TestFileRegistrationHooksCanDisableAndResetDefaults(t *testing.T) {
	client := &Client{}
	client.SetFileRegistrationHooks()

	tasks, err := client.fileRegistrationBackgroundTasks([]string{"hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("disabled hooks produced tasks: %#v", tasks)
	}

	client.ResetFileRegistrationHooks()
	tasks, err = client.fileRegistrationBackgroundTasks([]string{"hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("reset hooks produced %d tasks, want one immediate wake: %#v", len(tasks), tasks)
	}
	if task := tasks[0]; task.Kind != BackgroundMediaMetadataSweepTaskKind || task.OperationBinding != BackgroundOperationReuseActive {
		t.Fatalf("reset hooks produced unexpected task: %#v", task)
	}
}

func TestDefaultFileRegistrationHookSkipsKnownContent(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	client.SetFileRegistrationHooks()
	path := filepath.Join(t.TempDir(), "existing.jpg")
	if _, err := client.TagKnownFiles([]types.LocationInfo{{Path: path, Hash: "existing-hash", Size: 1, ModTime: 1}}, nil, nil); err != nil {
		t.Fatalf("register existing content: %v", err)
	}

	client.ResetFileRegistrationHooks()
	tasks, err := client.fileRegistrationBackgroundTasks([]string{"existing-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("known content produced metadata sweep tasks: %#v", tasks)
	}

	tasks, err = client.fileRegistrationBackgroundTasks([]string{"new-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].Kind != BackgroundMediaMetadataSweepTaskKind {
		t.Fatalf("new content produced tasks %#v, want one metadata sweep wake", tasks)
	}
}

func TestMediaMetadataRegistrationTasksUsesWakeOnlySignal(t *testing.T) {
	tasks, err := mediaMetadataRegistrationTasks(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("no-registration signal produced tasks: %#v", tasks)
	}

	tasks, err = mediaMetadataRegistrationTasks(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("registration signal produced %d tasks, want one immediate wake", len(tasks))
	}
	task := tasks[0]
	if task.Kind != BackgroundMediaMetadataSweepTaskKind || task.OperationBinding != BackgroundOperationReuseActive {
		t.Fatalf("registration signal produced unexpected task: %#v", task)
	}
	if task.Operation == nil || task.Operation.Kind != BackgroundMediaMetadataSweepOperationKind || !task.Operation.Visible {
		t.Fatalf("registration signal produced unexpected operation: %#v", task.Operation)
	}
	if !task.AvailableAt.IsZero() {
		t.Fatalf("registration wake is delayed until %v", task.AvailableAt)
	}
}
