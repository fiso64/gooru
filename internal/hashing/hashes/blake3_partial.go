package hashes

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

const (
	// numChunks is the number of pieces to sample from the file.
	numChunks = 10
	// chunkSize is the size of each piece to sample.
	chunkSize = 64 * 1024 // 64KB
)

// partialHashThreshold is the minimum file size to apply partial hashing.
// Files smaller than this will be hashed completely for maximum reliability.
const partialHashThreshold = numChunks * chunkSize // 640KB

// HashFile computes a partial hash for large files, or a full hash for small files.
// For large files, the identity is composed of the file size and hashes of strategic chunks.
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
	fileSize := info.Size()

	hasher := blake3.New()

	// For files below the threshold, hash the entire content for maximum reliability.
	if fileSize < partialHashThreshold {
		if _, err := io.Copy(hasher, file); err != nil {
			return "", err
		}
		hashBytes := hasher.Sum(nil)
		return hex.EncodeToString(hashBytes), nil
	}

	// For larger files, perform partial hashing.
	// The hash is prefixed with the file size to ensure that two files with
	// identical sampled chunks but different sizes (e.g., due to truncation or padding)
	// produce different final hashes.
	_, err = fmt.Fprintf(hasher, "%d:", fileSize)
	if err != nil {
		return "", err
	}

	// Calculate equally spaced offsets for the chunks.
	// The first chunk is at the beginning, the last is at the end,
	// and the rest are distributed evenly in between.
	offsets := make([]int64, numChunks)
	if numChunks > 1 {
		// The total span of data from the start of the first chunk to the start of the last chunk.
		span := fileSize - chunkSize
		// The distance between the start of each chunk.
		step := span / int64(numChunks-1)
		for i := int64(0); i < numChunks; i++ {
			offsets[i] = i * step
		}
		// Ensure the last chunk is exactly at the end of the file.
		offsets[numChunks-1] = fileSize - chunkSize
	} else if numChunks == 1 {
		// Edge case for a single chunk: place it in the middle.
		offsets[0] = (fileSize - chunkSize) / 2
	}

	buf := make([]byte, chunkSize)
	for _, offset := range offsets {
		_, err := file.Seek(offset, io.SeekStart)
		if err != nil {
			return "", fmt.Errorf("failed to seek to offset %d: %w", offset, err)
		}
		_, err = io.ReadFull(file, buf)
		if err != nil {
			return "", fmt.Errorf("failed to read full chunk at offset %d: %w", offset, err)
		}

		if _, err = hasher.Write(buf); err != nil {
			return "", err
		}
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}