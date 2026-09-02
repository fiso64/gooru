package hashes

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

const (
	numChunks = 10
	chunkSize = 64 * 1024
)

const partialHashThreshold = numChunks * chunkSize

// HashSource computes the configured partial-content identity directly from a
// random-access source. The algorithm is byte-for-byte identical to HashFile:
// small files are hashed completely, while large files include size plus ten
// evenly distributed 64 KiB samples.
func HashSource(source io.ReaderAt, size int64) (string, error) {
	if source == nil {
		return "", fmt.Errorf("hash source is required")
	}
	if size < 0 {
		return "", fmt.Errorf("hash source size must be non-negative")
	}
	if size < partialHashThreshold {
		return HashSourceFull(source, size)
	}

	hasher := blake3.New()
	if _, err := fmt.Fprintf(hasher, "%d:", size); err != nil {
		return "", err
	}

	offsets := make([]int64, numChunks)
	if numChunks > 1 {
		span := size - chunkSize
		step := span / int64(numChunks-1)
		for i := int64(0); i < numChunks; i++ {
			offsets[i] = i * step
		}
		offsets[numChunks-1] = size - chunkSize
	} else if numChunks == 1 {
		offsets[0] = (size - chunkSize) / 2
	}

	buf := make([]byte, chunkSize)
	for _, offset := range offsets {
		reader := io.NewSectionReader(source, offset, chunkSize)
		if _, err := io.ReadFull(reader, buf); err != nil {
			return "", fmt.Errorf("failed to read full chunk at offset %d: %w", offset, err)
		}
		if _, err := hasher.Write(buf); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// HashFile computes a partial hash for large files, or a full hash for small files.
func HashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	return HashSource(file, info.Size())
}
