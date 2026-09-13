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
	if analysisPath == "" {
		analysisPath = fileStoragePath(file)
	}
	if !l.encryption.Enabled && filepath.Clean(analysisPath) == filepath.Clean(file.Path) {
		if pathProvider, ok := provider.(MediaMetadataPathProvider); ok {
			return pathProvider.Metadata(ctx, file, mediaType, mediaKind)
		}
	}
	source, size, _, err := l.openUploadAnalysisSource(analysisPath)
	if err != nil {
		return MediaMetadata{}, err
	}
	defer source.Close()
	return provider.MetadataFromSource(ctx, file, source, size, mediaType, mediaKind)
}
