package encryptedfile

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"math"
)

// EncryptStream writes an encrypted file from a plaintext stream whose total
// size is not known in advance. Plaintext is never materialized in the output:
// chunks are first written under a temporary random nonce prefix, then
// re-authenticated in place under a fresh final prefix once the size is known.
// The completed output is the same versioned format produced by Encrypt.
//
// dst is expected to be a newly created seekable file. If this function
// returns an error, the caller must discard dst; the partially written output
// is intentionally not guaranteed to be a valid encrypted file.
func EncryptStream(dst io.ReadWriteSeeker, src io.Reader, key []byte) (int64, error) {
	if dst == nil || src == nil {
		return 0, fmt.Errorf("%w: nil stream", ErrInvalidFormat)
	}
	aead, err := newAEAD(key)
	if err != nil {
		return 0, err
	}

	provisionalCore, provisionalPrefix, err := newHeaderCore(0)
	if err != nil {
		return 0, err
	}
	provisionalTag := aead.Seal(nil, nonce(provisionalPrefix, math.MaxUint64), nil, provisionalCore)
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := dst.Write(provisionalCore); err != nil {
		return 0, err
	}
	if _, err := dst.Write(provisionalTag); err != nil {
		return 0, err
	}

	buf := make([]byte, ChunkSize)
	var plaintextSize int64
	var chunkIndex uint64
	for {
		n, readErr := io.ReadFull(src, buf)
		if n > 0 {
			if plaintextSize > math.MaxInt64-int64(n) {
				return 0, fmt.Errorf("%w: size overflow", ErrPlaintextSize)
			}
			plain := buf[:n]
			sealed := aead.Seal(nil, nonce(provisionalPrefix, chunkIndex), plain, chunkAAD(provisionalCore, chunkIndex))
			if _, err := dst.Write(sealed); err != nil {
				return 0, err
			}
			plaintextSize += int64(n)
			chunkIndex++
		}
		switch readErr {
		case nil:
			continue
		case io.EOF, io.ErrUnexpectedEOF:
			return finalizeStream(dst, aead, provisionalCore, provisionalPrefix, plaintextSize)
		default:
			return 0, readErr
		}
	}
}

func finalizeStream(dst io.ReadWriteSeeker, aead interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
	Overhead() int
}, provisionalCore []byte, provisionalPrefix [noncePrefixSize]byte, plaintextSize int64) (int64, error) {
	finalCore, finalPrefix, err := newHeaderCore(plaintextSize)
	if err != nil {
		return 0, err
	}
	for bytes.Equal(finalPrefix[:], provisionalPrefix[:]) {
		if _, err := rand.Read(finalPrefix[:]); err != nil {
			return 0, err
		}
		copy(finalCore[24:40], finalPrefix[:])
	}

	chunks := chunkCount(plaintextSize)
	for chunkIndex := uint64(0); chunkIndex < chunks; chunkIndex++ {
		plainLength := int64(ChunkSize)
		if remaining := plaintextSize - int64(chunkIndex)*ChunkSize; remaining < plainLength {
			plainLength = remaining
		}
		cipherLength := plainLength + int64(aead.Overhead())
		cipherOffset := int64(headerSize) + int64(chunkIndex)*(ChunkSize+int64(aead.Overhead()))
		if _, err := dst.Seek(cipherOffset, io.SeekStart); err != nil {
			return 0, err
		}
		sealed := make([]byte, int(cipherLength))
		if _, err := io.ReadFull(dst, sealed); err != nil {
			return 0, fmt.Errorf("%w: reread provisional chunk: %v", ErrInvalidFormat, err)
		}
		plain, err := aead.Open(nil, nonce(provisionalPrefix, chunkIndex), sealed, chunkAAD(provisionalCore, chunkIndex))
		if err != nil {
			return 0, ErrAuthentication
		}
		finalSealed := aead.Seal(nil, nonce(finalPrefix, chunkIndex), plain, chunkAAD(finalCore, chunkIndex))
		if len(finalSealed) != len(sealed) {
			return 0, fmt.Errorf("%w: unexpected chunk size", ErrInvalidFormat)
		}
		if _, err := dst.Seek(cipherOffset, io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := dst.Write(finalSealed); err != nil {
			return 0, err
		}
	}

	finalTag := aead.Seal(nil, nonce(finalPrefix, math.MaxUint64), nil, finalCore)
	if _, err := dst.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := dst.Write(finalCore); err != nil {
		return 0, err
	}
	if _, err := dst.Write(finalTag); err != nil {
		return 0, err
	}
	expectedSize := int64(headerSize) + plaintextSize + int64(chunks)*int64(aead.Overhead())
	if _, err := dst.Seek(expectedSize, io.SeekStart); err != nil {
		return 0, err
	}
	return plaintextSize, nil
}
