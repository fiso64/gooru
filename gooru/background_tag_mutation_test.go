package gooru

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gooru.local/types"
)

func newBackgroundTagMutationTestClient(t *testing.T) *Client {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "gooru.db")
	if err := Init(dbPath, types.StrategyFull, false); err != nil {
		t.Fatalf("init database: %v", err)
	}
	client, err := New(dbPath, false)
	if err != nil {
		t.Fatalf("open client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func writeBackgroundTagMutationTestFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestBackgroundTagMutationResultNotifiesSubscribers(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "body")
	if _, err := client.TagFiles([]string{path}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	changes, unsubscribe := client.SubscribeBackgroundOperationChanges()
	defer unsubscribe()
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation: "add", Selector: map[string]string{"query": "group:one"},
		Tags: []string{"reviewed"}, Query: "group:one", MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("admission did not notify background operation subscribers")
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("execute mutation: %v", err)
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("committed mutation result did not notify background operation subscribers")
	}
}

func TestBackgroundTagMutationQueryUsesAdmissionSnapshot(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	first := writeBackgroundTagMutationTestFile(t, dir, "first.jpg", "first")
	if _, err := client.TagFiles([]string{first}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("tag first file: %v", err)
	}

	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "add",
		Selector:   map[string]string{"query": "group:one"},
		Tags:       []string{"reviewed"},
		Query:      "group:one",
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create background mutation: %v", err)
	}

	second := writeBackgroundTagMutationTestFile(t, dir, "second.jpg", "second")
	if _, err := client.TagFiles([]string{second}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("tag second file: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("execute snapshotted query: %v", err)
	}

	firstTags, _, err := client.GetTagsForFile(first, false)
	if err != nil {
		t.Fatalf("read first tags: %v", err)
	}
	secondTags, _, err := client.GetTagsForFile(second, false)
	if err != nil {
		t.Fatalf("read second tags: %v", err)
	}
	if !containsBackgroundTag(firstTags, "reviewed") {
		t.Fatalf("snapshotted file did not receive reviewed tag: %v", firstTags)
	}
	if containsBackgroundTag(secondTags, "reviewed") {
		t.Fatalf("post-admission query match was mutated: %v", secondTags)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read background mutation: found=%v err=%v", found, err)
	}
	if state.MatchedFiles != 1 || state.AffectedCount != 1 || !state.ResultReady {
		t.Fatalf("unexpected result state: %+v", state)
	}
}

func TestBackgroundTagMutationReplayReturnsStoredResult(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "body")
	if _, err := client.TagFiles([]string{path}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "add",
		Selector:   map[string]string{"query": "group:one"},
		Tags:       []string{"reviewed"},
		Query:      "group:one",
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("first execution: %v", err)
	}
	firstState, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read first result: found=%v err=%v", found, err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("replay execution: %v", err)
	}
	secondState, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read replay result: found=%v err=%v", found, err)
	}
	firstJSON, _ := json.Marshal(firstState)
	secondJSON, _ := json.Marshal(secondState)
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("replay changed exact result:\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
}

func TestBackgroundTagMutationCancellationRollsBackTagWrite(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	path := writeBackgroundTagMutationTestFile(t, dir, "file.jpg", "body")
	if _, err := client.TagFiles([]string{path}, []string{"group:one"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:   "add",
		Selector:   map[string]string{"query": "group:one"},
		Tags:       []string{"reviewed"},
		Query:      "group:one",
		MaxPending: 8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	canceled, err := client.CancelBackgroundOperation(operation.ID)
	if err != nil || !canceled {
		t.Fatalf("cancel mutation: canceled=%v err=%v", canceled, err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err == nil {
		t.Fatal("expected canceled operation execution to fail")
	}
	tags, _, err := client.GetTagsForFile(path, false)
	if err != nil {
		t.Fatalf("read tags after cancellation: %v", err)
	}
	if containsBackgroundTag(tags, "reviewed") {
		t.Fatalf("canceled mutation committed domain write: %v", tags)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read canceled mutation: found=%v err=%v", found, err)
	}
	if state.ResultReady {
		t.Fatalf("canceled mutation published result: %+v", state)
	}
}

func TestBackgroundTagMutationPathResultPersistsMoveNotification(t *testing.T) {
	client := newBackgroundTagMutationTestClient(t)
	dir := t.TempDir()
	oldPath := writeBackgroundTagMutationTestFile(t, dir, "old.jpg", "body")
	if _, err := client.TagFiles([]string{oldPath}, []string{"initial"}, nil, false); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	files, err := client.GetAllFilesInfo()
	if err != nil || len(files) != 1 {
		t.Fatalf("list files: files=%v err=%v", files, err)
	}
	publicID := client.PublicFileID(files[0].ID)
	if publicID == "" {
		t.Fatal("missing public file id")
	}
	operation, err := client.CreateBackgroundTagMutation(BackgroundTagMutationRequest{
		Mutation:       "add",
		Selector:       map[string][]string{"file_ids": []string{publicID}},
		Tags:           []string{"reviewed"},
		FileIDs:        []string{publicID},
		FileIDSelector: true,
		MaxPending:     8,
	})
	if err != nil {
		t.Fatalf("create mutation: %v", err)
	}
	newPath := filepath.Join(dir, "new.jpg")
	if err := os.Rename(oldPath, newPath); err != nil {
		t.Fatalf("move file: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationPaths(operation.ID, []string{newPath}); err != nil {
		t.Fatalf("execute path mutation: %v", err)
	}
	state, found, err := client.GetBackgroundTagMutation(operation.ID)
	if err != nil || !found {
		t.Fatalf("read mutation result: found=%v err=%v", found, err)
	}
	if !state.ResultReady || state.AffectedCount != 1 || len(state.Notifications) != 1 {
		t.Fatalf("unexpected move result: %+v", state)
	}
	notification := state.Notifications[0]
	if notification.Kind != types.NotificationKindMoveDetected || notification.OldPath != oldPath || notification.NewPath != newPath {
		t.Fatalf("unexpected move notification: %+v", notification)
	}
}

func containsBackgroundTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}
