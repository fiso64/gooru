package hashing

import (
	"encoding/hex"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

// HashFile computes the BLAKE3 hash of a file and returns it as a hex string.
func HashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes), nil
}
