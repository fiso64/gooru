package managedfile

import (
	"errors"
	"io"

	"gooru.local/internal/encryptedfile"
)

// ErrTooLarge indicates that the plaintext input exceeded the configured limit.
var ErrTooLarge = errors.New("managed file exceeds size limit")

// Writer persists managed content according to a storage policy selected at the
// composition boundary. Callers provide plaintext and do not need to know
// whether the on-disk representation is encrypted.
type Writer struct {
	key []byte
}

// NewFilesystem returns a writer that persists plaintext content.
func NewFilesystem() *Writer {
	return &Writer{}
}

// NewProtected returns a writer that persists authenticated encrypted content.
// The key is copied so callers do not retain ownership of the writer's key
// material through a shared backing slice.
func NewProtected(key []byte) *Writer {
	return &Writer{key: append([]byte(nil), key...)}
}

// Write replaces dst with the logical plaintext read from src. limit is a
// plaintext byte limit; values <= 0 mean unlimited. dst must be a newly-created
// seekable file because protected writes finalize authenticated chunks in place.
func (w *Writer) Write(dst io.ReadWriteSeeker, src io.Reader, limit int64) (int64, error) {
	if len(w.key) == 0 {
		return copyLimited(dst, src, limit)
	}

	input := src
	if limit > 0 {
		input = &io.LimitedReader{R: src, N: limit + 1}
	}
	size, err := encryptedfile.EncryptStream(dst, input, w.key)
	if err != nil {
		return size, err
	}
	if limit > 0 && size > limit {
		return size, ErrTooLarge
	}
	return size, nil
}

func copyLimited(dst io.Writer, src io.Reader, limit int64) (int64, error) {
	if limit <= 0 {
		return io.Copy(dst, src)
	}
	written, err := io.Copy(dst, io.LimitReader(src, limit+1))
	if err != nil {
		return written, err
	}
	if written > limit {
		return written, ErrTooLarge
	}
	return written, nil
}
