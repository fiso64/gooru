package gooru

import (
	"testing"

	"gooru.local/types"
)

func TestTagKnownFilesWithBackgroundTasksRollsBackRegistrationWhenEnqueueFails(t *testing.T) {
	client := newBackgroundEnqueueTestClient(t)
	location := types.LocationInfo{Path: "/library/item.jpg", Hash: "hash-atomic-import", Size: 12, ModTime: 34, Extension: ".jpg"}
	_, err := client.TagKnownFilesWithBackgroundTasks([]types.LocationInfo{location}, nil, []BackgroundTaskRequest{{
		DedupeKey: "invalid-without-kind",
	}}, nil)
	if err == nil {
		t.Fatal("expected invalid background task to fail")
	}
	exists, lookupErr := client.ContentExists(location.Hash)
	if lookupErr != nil {
		t.Fatalf("ContentExists: %v", lookupErr)
	}
	if exists {
		t.Fatal("content registration committed despite background enqueue failure")
	}
}
