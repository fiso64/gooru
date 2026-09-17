package database

import "testing"

func TestBackgroundOperationResultPersistsCompactSummary(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{
		ID:      "op-summary",
		Kind:    "upload_import",
		Visible: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := []byte(`{"affected_count":2,"files":[{"status":"imported"},{"status":"imported"},{"status":"error"}]}`)
	if err := store.SetBackgroundOperationResult(op.ID, result); err != nil {
		t.Fatalf("SetBackgroundOperationResult: %v", err)
	}
	summaries, err := store.GetBackgroundOperationResultSummaries([]string{op.ID, "missing"})
	if err != nil {
		t.Fatalf("GetBackgroundOperationResultSummaries: %v", err)
	}
	summary, ok := summaries[op.ID]
	if !ok {
		t.Fatalf("summary missing: %+v", summaries)
	}
	if summary.Outcome != "partial_success" || summary.AffectedCount == nil || *summary.AffectedCount != 2 || summary.FailedCount == nil || *summary.FailedCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if _, exists := summaries["missing"]; exists {
		t.Fatalf("missing operation unexpectedly summarized: %+v", summaries)
	}
}

func TestBackgroundOperationResultSummaryUsesMatchedFiles(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{ID: "op-tag-summary", Kind: "tag_mutation"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetBackgroundOperationResult(op.ID, []byte(`{"matched_files":9}`)); err != nil {
		t.Fatalf("SetBackgroundOperationResult: %v", err)
	}
	summaries, err := store.GetBackgroundOperationResultSummaries([]string{op.ID})
	if err != nil {
		t.Fatal(err)
	}
	summary := summaries[op.ID]
	if summary.Outcome != "success" || summary.AffectedCount == nil || *summary.AffectedCount != 9 || summary.FailedCount == nil || *summary.FailedCount != 0 {
		t.Fatalf("summary = %+v", summary)
	}
}
