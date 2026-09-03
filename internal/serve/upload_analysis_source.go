package serve

import (
	"context"
	"fmt"
	"io"
	"os"

	"gooru.local/types"
)

type uploadAnalysisSource interface {
	io.ReaderAt
	io.Closer
}

func openUploadAnalysisSource(path string) (uploadAnalysisSource, int64, int64, error) {
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

func importedMediaMetadata(ctx context.Context, provider MediaMetadataProvider, file types.FileInfo, analysisPath string, mediaType string, mediaKind string) (MediaMetadata, error) {
	if sourceProvider, ok := provider.(MediaMetadataSourceProvider); ok && analysisPath != "" {
		source, size, _, err := openUploadAnalysisSource(analysisPath)
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
