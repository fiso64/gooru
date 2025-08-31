package hashing

import "gooru.local/gooru/internal/hashing/hashes"

// HashFile computes a partial hash for large files, or a full hash for small files.
// This is the default hashing strategy for Gooru.
func HashFile(filePath string) (string, error) {
	return hashes.HashFile(filePath)
}