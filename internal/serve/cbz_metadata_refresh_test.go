package serve

import (
	"context"
	"path/filepath"
	"testing"

	"gooru.local/types"
)

type cbzMetadataRefreshLibrary struct {
	metadata      MediaMetadata
	metadataCalls int
}

func (l *cbzMetadataRefreshLibrary) ListFiles(context.Context, string) ([]types.FileInfo, error) {
	return nil, nil
}
func (l *cbzMetadataRefreshLibrary) GetFile(context.Context, int64) (types.FileInfo, error) {
	return types.FileInfo{}, nil
}
func (l *cbzMetadataRefreshLibrary) ListTags(context.Context, bool, int) ([]TagDTO, error) {
	return nil, nil
}
func (l *cbzMetadataRefreshLibrary) ListFilesSearch(context.Context, string, Page, string, string) (PageResult[types.FileInfo], error) {
	return PageResult[types.FileInfo]{}, nil
}
func (l *cbzMetadataRefreshLibrary) LibraryCount(context.Context) (int, error) { return 0, nil }
func (l *cbzMetadataRefreshLibrary) KindFacets(context.Context, string) ([]FacetValueDTO, error) {
	return nil, nil
}
func (l *cbzMetadataRefreshLibrary) TagSuggestions(context.Context, string, string, int) ([]TagDTO, error) {
	return nil, nil
}
func (l *cbzMetadataRefreshLibrary) TagNamespaces(context.Context) ([]string, error) { return nil, nil }
func (l *cbzMetadataRefreshLibrary) FileMetadata(context.Context, int64) (MediaMetadata, error) {
	l.metadataCalls++
	return l.metadata, nil
}

func TestFileDTORefreshesLegacyComicMetadataMissingCoverDimensions(t *testing.T) {
	width, height, pages := 600, 900, 24
	library := &cbzMetadataRefreshLibrary{metadata: MediaMetadata{ImageWidth: &width, ImageHeight: &height, PageCount: &pages}}
	server := &Server{cfg: DefaultConfig(filepath.Join(t.TempDir(), "gooru.db")), library: library}
	file := types.FileInfo{
		ID:   42,
		Path: filepath.Join(t.TempDir(), "book.cbz"),
		Metadata: &types.MediaMetadata{
			LocationID: 42,
			PageCount:  &pages,
		},
	}

	dto := server.fileDTO(context.Background(), file, false)
	if library.metadataCalls != 1 {
		t.Fatalf("expected one bounded lazy metadata refresh, got %d", library.metadataCalls)
	}
	if dto.Metadata.ImageWidth == nil || *dto.Metadata.ImageWidth != width || dto.Metadata.ImageHeight == nil || *dto.Metadata.ImageHeight != height {
		t.Fatalf("expected refreshed CBZ cover dimensions, got %+v", dto.Metadata)
	}
}

func TestFileDTODoesNotRefreshCompleteComicMetadata(t *testing.T) {
	width, height, pages := 600, 900, 24
	library := &cbzMetadataRefreshLibrary{}
	server := &Server{cfg: DefaultConfig(filepath.Join(t.TempDir(), "gooru.db")), library: library}
	file := types.FileInfo{
		ID:   42,
		Path: filepath.Join(t.TempDir(), "book.cbz"),
		Metadata: &types.MediaMetadata{
			LocationID:  42,
			ImageWidth:  &width,
			ImageHeight: &height,
			PageCount:   &pages,
		},
	}

	dto := server.fileDTO(context.Background(), file, false)
	if library.metadataCalls != 0 {
		t.Fatalf("complete persisted CBZ metadata should not be re-read, got %d calls", library.metadataCalls)
	}
	if dto.Metadata.ImageWidth == nil || *dto.Metadata.ImageWidth != width || dto.Metadata.ImageHeight == nil || *dto.Metadata.ImageHeight != height {
		t.Fatalf("expected persisted CBZ cover dimensions, got %+v", dto.Metadata)
	}
}
