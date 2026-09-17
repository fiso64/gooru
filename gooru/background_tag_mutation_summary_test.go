package gooru

import "testing"

func TestBackgroundTagMutationSummaryIncludesDurableAffectedCount(t *testing.T) {
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
		t.Fatalf("create background mutation: %v", err)
	}
	if err := client.ExecuteBackgroundTagMutationQuery(operation.ID); err != nil {
		t.Fatalf("execute background mutation: %v", err)
	}

	summaries, err := client.GetBackgroundOperationResultSummaries([]string{operation.ID})
	if err != nil {
		t.Fatalf("read background operation summaries: %v", err)
	}
	summary, found := summaries[operation.ID]
	if !found {
		t.Fatal("missing background operation result summary")
	}
	if summary.Outcome != "success" {
		t.Fatalf("unexpected summary outcome: %q", summary.Outcome)
	}
	if summary.AffectedCount == nil || *summary.AffectedCount != 1 {
		t.Fatalf("unexpected summary affected count: %v", summary.AffectedCount)
	}
	if summary.FailedCount == nil || *summary.FailedCount != 0 {
		t.Fatalf("unexpected summary failed count: %v", summary.FailedCount)
	}
}
