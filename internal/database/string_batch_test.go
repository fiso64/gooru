package database

import (
	"fmt"
	"strings"
	"testing"
)

func TestStringBindBatchesRespectsBindBudget(t *testing.T) {
	values := make([]string, maxVars+2)
	for i := range values {
		values[i] = fmt.Sprintf("value-%04d", i)
	}

	offset := 0
	batches := 0
	for placeholders, args := range stringBindBatches(values) {
		batches++
		wantSize := maxVars
		if batches == 2 {
			wantSize = 2
		}
		if len(args) != wantSize {
			t.Fatalf("batch %d args = %d, want %d", batches, len(args), wantSize)
		}
		if got := strings.Count(placeholders, "?"); got != wantSize {
			t.Fatalf("batch %d placeholders = %d, want %d", batches, got, wantSize)
		}
		for i, arg := range args {
			if arg != values[offset+i] {
				t.Fatalf("batch %d arg %d = %v, want %q", batches, i, arg, values[offset+i])
			}
		}
		offset += len(args)
	}

	if batches != 2 {
		t.Fatalf("batches = %d, want 2", batches)
	}
	if offset != len(values) {
		t.Fatalf("consumed values = %d, want %d", offset, len(values))
	}
}
