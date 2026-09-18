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

func TestValueBindBatchesSupportsRowPlaceholders(t *testing.T) {
	values := make([]string, maxVars+2)
	for i := range values {
		values[i] = fmt.Sprintf("value-%04d", i)
	}

	offset := 0
	batches := 0
	for placeholders, args := range bindValueBatches(values, "(?)") {
		batches++
		wantSize := maxVars
		if batches == 2 {
			wantSize = 2
		}
		wantPlaceholders := strings.TrimSuffix(strings.Repeat("(?),", wantSize), ",")
		if placeholders != wantPlaceholders {
			t.Fatalf("batch %d placeholders differ: got length %d want length %d", batches, len(placeholders), len(wantPlaceholders))
		}
		if len(args) != wantSize {
			t.Fatalf("batch %d args = %d, want %d", batches, len(args), wantSize)
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

func TestContentTagPairBindBatchesRespectsBindBudget(t *testing.T) {
	const pairCount = maxVars/2 + 1
	pairs := make([]ContentTagPair, pairCount)
	for i := range pairs {
		pairs[i] = ContentTagPair{ContentHash: fmt.Sprintf("hash-%04d", i), TagID: int64(i + 1)}
	}

	offset := 0
	batches := 0
	for placeholders, args := range contentTagPairBindBatches(pairs) {
		batches++
		wantPairs := maxVars / 2
		if batches == 2 {
			wantPairs = 1
		}
		if len(args) != wantPairs*2 {
			t.Fatalf("batch %d args = %d, want %d", batches, len(args), wantPairs*2)
		}
		if got := strings.Count(placeholders, "?"); got != wantPairs*2 {
			t.Fatalf("batch %d placeholders = %d, want %d", batches, got, wantPairs*2)
		}
		for i := 0; i < wantPairs; i++ {
			pair := pairs[offset+i]
			if args[i*2] != pair.ContentHash || args[i*2+1] != pair.TagID {
				t.Fatalf("batch %d pair %d mismatch", batches, i)
			}
		}
		offset += wantPairs
	}

	if batches != 2 {
		t.Fatalf("batches = %d, want 2", batches)
	}
	if offset != len(pairs) {
		t.Fatalf("consumed pairs = %d, want %d", offset, len(pairs))
	}
}
