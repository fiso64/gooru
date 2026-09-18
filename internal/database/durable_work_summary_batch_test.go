package database

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"testing"
)

func TestBackgroundOperationResultSummariesRespectBindBudget(t *testing.T) {
	store, db := newDurableWorkTestDB(t)
	store.DB = db

	var queryLog bytes.Buffer
	store.logger = log.New(&queryLog, "", 0)
	operationIDs := make([]string, maxVars+1)
	for i := range operationIDs {
		operationIDs[i] = fmt.Sprintf("missing-%d", i)
	}
	summaries, err := store.GetBackgroundOperationResultSummaries(operationIDs)
	if err != nil {
		t.Fatalf("GetBackgroundOperationResultSummaries: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("unexpected summaries: %+v", summaries)
	}

	logged := queryLog.String()
	if got := strings.Count(logged, "FROM background_operations"); got != 2 {
		t.Fatalf("summary queries = %d, want 2\n%s", got, logged)
	}
	if !strings.Contains(logged, fmt.Sprintf("-- ARGS: %d bound values redacted", maxVars)) {
		t.Fatalf("missing max-sized batch in query log:\n%s", logged)
	}
	if !strings.Contains(logged, "-- ARGS: 1 bound values redacted") {
		t.Fatalf("missing remainder batch in query log:\n%s", logged)
	}
}
