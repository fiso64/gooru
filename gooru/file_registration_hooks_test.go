package gooru

import (
	"reflect"
	"testing"
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
