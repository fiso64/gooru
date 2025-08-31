package hashes

import (
	"encoding/hex"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

// HashFileFull computes the BLAKE3 hash of a file and returns it as a hex string.
func HashFileFull(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Using a pooled hasher can improve performance by reducing allocations.
	hasher := blake3.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}