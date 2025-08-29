// package hashing

// import (
// 	"encoding/hex"
// 	"io"
// 	"os"

// 	"github.com/zeebo/blake3"
// )

// HashFile computes the BLAKE3 hash of a file and returns it as a hex string.
// func HashFile(filePath string) (string, error) {
// 	file, err := os.Open(filePath)
// 	if err != nil {
// 		return "", err
// 	}
// 	defer file.Close()

// 	// Using a pooled hasher can improve performance by reducing allocations.
// 	hasher := blake3.New()
// 	if _, err := io.Copy(hasher, file); err != nil {
// 		return "", err
// 	}

// 	hashBytes := hasher.Sum(nil)
// 	return hex.EncodeToString(hashBytes), nil
// }


package hashing

import (
	"crypto/rand"
	"encoding/hex"
)

// HashFile computes the BLAKE3 hash of a file and returns it as a hex string.
// DEBUG: This is a temporary implementation that returns a random string
// to isolate hashing performance from database performance.
func HashFile(filePath string) (string, error) {
	// This avoids disk I/O for hashing.
	randomBytes := make([]byte, 32) // BLAKE3 default output is 32 bytes
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}