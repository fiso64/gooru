package encryptedfile

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// IsEncryptedFile identifies the Gooru encrypted-file container without
// authenticating it. Callers that receive true must still use Open with the
// configured key before exposing plaintext.
func IsEncryptedFile(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer file.Close()

	prefix := make([]byte, len(magic)+1)
	if _, err := io.ReadFull(file, prefix); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return false, nil
		}
		return false, err
	}
	return bytes.Equal(prefix[:len(magic)], []byte(magic)) && prefix[len(magic)] == version, nil
}
