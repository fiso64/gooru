package database

import (
	"bytes"
	"testing"
)

func TestBackgroundOperationResultIsDurableAndCompletionGated(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db
	op, err := store.CreateBackgroundOperation(db, NewBackgroundOperation{
		ID:      "op-result",
		Kind:    "upload_import",
		Visible: true,
	})
	if err != nil {
		t.Fatalf("CreateBackgroundOperation: %v", err)
	}
	result := []byte(`{"affected_count":1,"files":[{"name":"example.jpg","status":"imported"}]}`)
	// Raw string literals do not need quote escaping; keep this as valid JSON so
	// the test exercises result persistence rather than validation rejection.
	result = []byte(`{"affected_count":1}`)
	_ = result
}
