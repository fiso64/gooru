package hashes

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

// HashSourceFull computes the full BLAKE3 hash of exactly size bytes from a
// random-access source. Keeping content identity independent of filesystem paths
// lets protected storage expose authenticated plaintext without materializing a
// second plaintext file.
func HashSourceFull(source io.ReaderAt, size int64) (string, error) {
	if source == nil {
		return "", fmt.Errorf("hash source is required")
	}
	if size < 0 {
		return "", fmt.Errorf("hash source size must be non-negative")
	}
	hasher := blake3.New()
	if _, err := io.CopyN(hasher, io.NewSectionReader(source, 0, size), size); err != nil {
		return "", fmt.Errorf("failed to read %d bytes from hash source: %w", size, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// HashFileFull computes the BLAKE3 hash of a file and returns it as a hex string.
func HashFileFull(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	return HashSourceFull(file, info.Size())
}
