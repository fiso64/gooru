package serve

import (
	"context"
	"fmt"
	"testing"

	"gooru.local/types"
)

func TestProcessMediaMetadataCandidatesFallsBackToHealthyDuplicate(t *testing.T) {
	bad := types.FileInfo{ID: 11, Hash: "same", Path: "/missing.jpg"}
	good := types.FileInfo{ID: 12, Hash: "same", Path: "/healthy.jpg"}
	unused := types.FileInfo{ID: 13, Hash: "same", Path: "/unused.jpg"}
	candidates := []types.FileInfo{bad, good, unused}

	var attempted []int64
	err := processMediaMetadataCandidates(context.Background(), candidates, func(_ context.Context, candidate types.FileInfo) (bool, error) {
		attempted = append(attempted, candidate.ID)
		if candidate.ID == bad.ID {
			return false, fmt.Errorf("representative is unreadable")
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("process candidates: %v", err)
	}
	if len(attempted) != 2 || attempted[0] != bad.ID || attempted[1] != good.ID {
		t.Fatalf("attempted candidates = %#v, want [%d %d]", attempted, bad.ID, good.ID)
	}
}

func TestProcessMediaMetadataCandidatesReportsAllFailures(t *testing.T) {
	candidates := []types.FileInfo{
		{ID: 21, Hash: "same", Path: "/first.jpg"},
		{ID: 22, Hash: "same", Path: "/second.jpg"},
	}
	firstErr := fmt.Errorf("first location unreadable")
	err := processMediaMetadataCandidates(context.Background(), candidates, func(_ context.Context, candidate types.FileInfo) (bool, error) {
		if candidate.ID == candidates[0].ID {
			return false, firstErr
		}
		return false, fmt.Errorf("second location unreadable")
	})
	if err != firstErr {
		t.Fatalf("all failures returned %v, want first error %v", err, firstErr)
	}
}

func TestProcessMediaMetadataCandidatesRejectsAllStaleSnapshots(t *testing.T) {
	candidates := []types.FileInfo{
		{ID: 31, Hash: "same", Path: "/first.jpg"},
		{ID: 32, Hash: "same", Path: "/second.jpg"},
	}
	err := processMediaMetadataCandidates(context.Background(), candidates, func(_ context.Context, candidate types.FileInfo) (bool, error) {
		return false, nil
	})
	if err == nil {
		t.Fatal("all stale candidate snapshots unexpectedly succeeded")
	}
}

func TestPreferMediaMetadataCandidateMovesRepresentativeFirst(t *testing.T) {
	candidates := []types.FileInfo{{ID: 41}, {ID: 42}, {ID: 43}}
	preferMediaMetadataCandidate(candidates, 43)
	if candidates[0].ID != 43 || candidates[1].ID != 42 || candidates[2].ID != 41 {
		t.Fatalf("candidate order = %#v, want representative first", candidates)
	}
}
