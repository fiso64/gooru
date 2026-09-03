package serve

import (
	"fmt"
	"io"
	"os"
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
