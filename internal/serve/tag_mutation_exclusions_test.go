package serve

import (
	"strings"
	"testing"
)

func TestTagMutationExclusionsRequireQuerySelector(t *testing.T) {
	err := validateTagMutationRequest(TagOperationAdd, TagMutationRequest{
		FileIDs:        []string{"file-a"},
		ExcludeFileIDs: []string{"file-b"},
		Tags:           []string{"reviewed"},
	})
	if err == nil || !strings.Contains(err.Error(), "exclude_file_ids requires a query selector") {
		t.Fatalf("expected query-only exclusion error, got %v", err)
	}
}

func TestTagMutationRejectsDuplicateExclusions(t *testing.T) {
	err := validateTagMutationRequest(TagOperationRemove, TagMutationRequest{
		Query:          "*",
		ExcludeFileIDs: []string{"file-a", "file-a"},
		Tags:           []string{"reviewed"},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate excluded file id") {
		t.Fatalf("expected duplicate exclusion error, got %v", err)
	}
}
