package serve

import (
	"context"
	"reflect"
	"testing"

	"gooru.local/types"
)

type removalLookupLibrary struct {
	emptyLibrary
	batchCalls     int
	pointCalls     int
	batchIDs       []string
	pointIDs       []string
	batchFiles     []types.FileInfo
	batchErr       error
}

func (l *removalLookupLibrary) PublicFileID(file types.FileInfo) string {
	return file.PublicID
}

func (l *removalLookupLibrary) GetFileByPublicID(_ context.Context, id string) (types.FileInfo, error) {
	l.pointCalls++
	l.pointIDs = append(l.pointIDs, id)
	return types.FileInfo{PublicID: id}, nil
}

func (l *removalLookupLibrary) DeleteFileByPublicID(context.Context, string) (bool, error) {
	return false, nil
}

func (l *removalLookupLibrary) GetFilesByPublicIDs(_ context.Context, ids []string) ([]types.FileInfo, error) {
	l.batchCalls++
	l.batchIDs = append([]string(nil), ids...)
	return append([]types.FileInfo(nil), l.batchFiles...), l.batchErr
}

type pointRemovalLookupLibrary struct {
	emptyLibrary
	pointCalls int
	pointIDs   []string
}

func (l *pointRemovalLookupLibrary) PublicFileID(file types.FileInfo) string {
	return file.PublicID
}

func (l *pointRemovalLookupLibrary) GetFileByPublicID(_ context.Context, id string) (types.FileInfo, error) {
	l.pointCalls++
	l.pointIDs = append(l.pointIDs, id)
	return types.FileInfo{PublicID: id}, nil
}

func (l *pointRemovalLookupLibrary) DeleteFileByPublicID(context.Context, string) (bool, error) {
	return false, nil
}

func TestResolveFileIDsUsesOneBatchLookupAfterExclusions(t *testing.T) {
	library := &removalLookupLibrary{batchFiles: []types.FileInfo{
		{PublicID: "file_a"},
		{PublicID: "file_b"},
	}}
	server := &Server{library: library}

	files, err := server.resolveFileIDs(
		context.Background(),
		[]string{"file_a", "file_skip", "file_b"},
		[]string{"file_skip"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if library.batchCalls != 1 {
		t.Fatalf("expected one batch lookup, got %d", library.batchCalls)
	}
	if library.pointCalls != 0 {
		t.Fatalf("expected no point lookups, got %d", library.pointCalls)
	}
	if want := []string{"file_a", "file_b"}; !reflect.DeepEqual(library.batchIDs, want) {
		t.Fatalf("unexpected batch IDs: got %v, want %v", library.batchIDs, want)
	}
	if got := []string{files[0].PublicID, files[1].PublicID}; !reflect.DeepEqual(got, library.batchIDs) {
		t.Fatalf("unexpected resolved order: got %v, want %v", got, library.batchIDs)
	}
}

func TestResolveFileIDsKeepsPointLookupFallbackForAlternateLibraries(t *testing.T) {
	library := &pointRemovalLookupLibrary{}
	server := &Server{library: library}

	files, err := server.resolveFileIDs(
		context.Background(),
		[]string{"file_a", "file_skip", "file_b"},
		[]string{"file_skip"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if library.pointCalls != 2 {
		t.Fatalf("expected two point lookups, got %d", library.pointCalls)
	}
	if want := []string{"file_a", "file_b"}; !reflect.DeepEqual(library.pointIDs, want) {
		t.Fatalf("unexpected point lookup IDs: got %v, want %v", library.pointIDs, want)
	}
	if got := []string{files[0].PublicID, files[1].PublicID}; !reflect.DeepEqual(got, library.pointIDs) {
		t.Fatalf("unexpected resolved order: got %v, want %v", got, library.pointIDs)
	}
}

func TestResolveFileIDsFallsBackWhenOptionalBatchResultIsIncomplete(t *testing.T) {
	library := &removalLookupLibrary{batchFiles: []types.FileInfo{{PublicID: "file_a"}}}
	server := &Server{library: library}

	files, err := server.resolveFileIDs(context.Background(), []string{"file_a", "file_b"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if library.batchCalls != 1 {
		t.Fatalf("expected one batch lookup, got %d", library.batchCalls)
	}
	if library.pointCalls != 2 {
		t.Fatalf("expected complete point-lookup fallback, got %d calls", library.pointCalls)
	}
	if want := []string{"file_a", "file_b"}; !reflect.DeepEqual(library.pointIDs, want) {
		t.Fatalf("unexpected fallback IDs: got %v, want %v", library.pointIDs, want)
	}
	if got := []string{files[0].PublicID, files[1].PublicID}; !reflect.DeepEqual(got, library.pointIDs) {
		t.Fatalf("unexpected fallback result order: got %v, want %v", got, library.pointIDs)
	}
}
