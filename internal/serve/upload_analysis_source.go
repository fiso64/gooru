package serve

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gooru.local/internal/encryptedfile"
	"gooru.local/types"
)

type uploadAnalysisSource interface {
	io.ReaderAt
	io.Closer
}

func (l *GooruLibrary) openUploadAnalysisSource(path string) (uploadAnalysisSource, int64, int64, error) {
	if l.encryption.Enabled {
		file, err := encryptedfile.Open(path, l.encryption.Key)
		if err != nil {
			return nil, 0, 0, err
		}
		return file, file.Size(), file.ModTime().Unix(), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, 0, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, 0, 0, fmt.Errorf("upload analysis source is a directory")
	}
	return file, info.Size(), info.ModTime().Unix(), nil
}

func (l *GooruLibrary) importedMediaMetadata(ctx context.Context, provider MediaMetadataProvider, file types.FileInfo, analysisPath string, mediaType string, mediaKind string) (MediaMetadata, error) {
	if uploadMediaMetadataDeferred(ctx) {
		return MediaMetadata{}, nil
	}
	if !l.encryption.Enabled && analysisPath != "" && filepath.Clean(analysisPath) == filepath.Clean(file.Path) {
		return provider.Metadata(ctx, file, mediaType, mediaKind)
	}
	if sourceProvider, ok := provider.(MediaMetadataSourceProvider); ok && analysisPath != "" {
		source, size, _, err := l.openUploadAnalysisSource(analysisPath)
		if err != nil {
			return MediaMetadata{}, err
		}
		defer source.Close()
		return sourceProvider.MetadataFromSource(ctx, file, source, size, mediaType, mediaKind)
	}
	analysisFile := file
	if analysisPath != "" {
		analysisFile.Path = analysisPath
	}
	return provider.Metadata(ctx, analysisFile, mediaType, mediaKind)
}
