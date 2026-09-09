package serve

import (
	"io"

	"gooru.local/types"
)

func (m *MediaService) thumbnailCBZFirstPageFile(file types.FileInfo, dst io.Writer, size int, format string) error {
	source, err := m.openMediaSource(fileStoragePath(file))
	if err != nil {
		return err
	}
	archive, err := openComicArchiveReader(file.Path, source, source.size, source.Close)
	if err != nil {
		_ = source.Close()
		return err
	}
	defer archive.Close()
	return thumbnailComicArchiveFirstPage(archive, dst, size, format)
}
