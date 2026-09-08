package encryptedfile

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	magic        = "GOORUENC"
	version byte = 1

	// ChunkSize bounds authentication/decryption work for random reads while
	// keeping ciphertext overhead small for large media files.
	ChunkSize = 1 << 20

	noncePrefixSize = 16
	headerCoreSize  = 40
	headerTagSize   = chacha20poly1305.Overhead
	headerSize      = headerCoreSize + headerTagSize
)

var (
	ErrInvalidKey     = errors.New("encrypted file key must be 32 bytes")
	ErrInvalidFormat  = errors.New("invalid encrypted file format")
	ErrAuthentication = errors.New("encrypted file authentication failed")
	ErrPlaintextSize  = errors.New("plaintext size does not match declared size")
)

type readAtCloser interface {
	io.ReaderAt
	io.Closer
}

// Encrypt writes a versioned, chunk-authenticated encrypted representation of
// exactly plaintextSize bytes from src. The header is authenticated separately
// so wrong keys and metadata tampering fail before any media bytes are exposed.
func Encrypt(dst io.Writer, src io.Reader, plaintextSize int64, key []byte) error {
	if plaintextSize < 0 {
		return fmt.Errorf("%w: negative size", ErrPlaintextSize)
	}
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}
	core, prefix, err := newHeaderCore(plaintextSize)
	if err != nil {
		return err
	}
	headerTag := aead.Seal(nil, nonce(prefix, math.MaxUint64), nil, core)
	if len(headerTag) != headerTagSize {
		return fmt.Errorf("%w: unexpected header tag size", ErrInvalidFormat)
	}
	if _, err := dst.Write(core); err != nil {
		return err
	}
	if _, err := dst.Write(headerTag); err != nil {
		return err
	}

	buf := make([]byte, ChunkSize)
	remaining := plaintextSize
	var chunkIndex uint64
	for remaining > 0 {
		want := int64(ChunkSize)
		if remaining < want {
			want = remaining
		}
		plain := buf[:int(want)]
		if _, err := io.ReadFull(src, plain); err != nil {
			return fmt.Errorf("%w: source ended early: %v", ErrPlaintextSize, err)
		}
		sealed := aead.Seal(nil, nonce(prefix, chunkIndex), plain, chunkAAD(core, chunkIndex))
		if _, err := dst.Write(sealed); err != nil {
			return err
		}
		remaining -= want
		chunkIndex++
	}

	var extra [1]byte
	n, err := io.ReadFull(src, extra[:])
	if n > 0 {
		return fmt.Errorf("%w: source contains more bytes than declared", ErrPlaintextSize)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// File exposes authenticated plaintext with Reader, ReaderAt, and Seeker
// semantics. Sequential reads retain one decrypted chunk per open file so small
// reads do not repeatedly authenticate the same 1 MiB chunk. Random ReadAt
// calls remain independent and decrypt only the chunks intersecting their range.
type File struct {
	file    readAtCloser
	aead    cipher.AEAD
	core    []byte
	prefix  [noncePrefixSize]byte
	size    int64
	modTime time.Time

	mu          sync.Mutex
	offset      int64
	cacheValid  bool
	cachedChunk uint64
	cachedPlain []byte
}

func Open(path string, key []byte) (*File, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	opened, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := opened.Stat()
	if err != nil {
		_ = opened.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = opened.Close()
		return nil, fmt.Errorf("%w: path is a directory", ErrInvalidFormat)
	}

	header := make([]byte, headerSize)
	if _, err := io.ReadFull(opened, header); err != nil {
		_ = opened.Close()
		return nil, fmt.Errorf("%w: truncated header", ErrInvalidFormat)
	}
	core := append([]byte(nil), header[:headerCoreSize]...)
	if string(core[:len(magic)]) != magic || core[len(magic)] != version {
		_ = opened.Close()
		return nil, ErrInvalidFormat
	}
	if !bytes.Equal(core[len(magic)+1:16], make([]byte, 7)) {
		_ = opened.Close()
		return nil, ErrInvalidFormat
	}
	var prefix [noncePrefixSize]byte
	copy(prefix[:], core[24:40])
	if _, err := aead.Open(nil, nonce(prefix, math.MaxUint64), header[headerCoreSize:], core); err != nil {
		_ = opened.Close()
		return nil, ErrAuthentication
	}

	plaintextSize := int64(binary.LittleEndian.Uint64(core[16:24]))
	if plaintextSize < 0 || plaintextSize > info.Size() {
		_ = opened.Close()
		return nil, ErrInvalidFormat
	}
	chunks := chunkCount(plaintextSize)
	overhead := int64(chunks) * int64(aead.Overhead())
	if overhead < 0 || plaintextSize > math.MaxInt64-int64(headerSize)-overhead {
		_ = opened.Close()
		return nil, ErrInvalidFormat
	}
	expectedSize := int64(headerSize) + plaintextSize + overhead
	if info.Size() != expectedSize {
		_ = opened.Close()
		return nil, fmt.Errorf("%w: ciphertext size mismatch", ErrInvalidFormat)
	}

	return &File{
		file:    opened,
		aead:    aead,
		core:    core,
		prefix:  prefix,
		size:    plaintextSize,
		modTime: info.ModTime(),
	}, nil
}

func (f *File) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	n, err := f.readAt(p, f.offset, true)
	f.offset += int64(n)
	return n, err
}

func (f *File) ReadAt(p []byte, off int64) (int, error) {
	return f.readAt(p, off, false)
}

func (f *File) readAt(p []byte, off int64, sequential bool) (int, error) {
	if off < 0 {
		return 0, errors.New("encrypted file: negative offset")
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= f.size {
		return 0, io.EOF
	}

	requested := len(p)
	available := f.size - off
	limit := requested
	if int64(limit) > available {
		limit = int(available)
	}

	written := 0
	var scratch []byte
	for written < limit {
		position := off + int64(written)
		chunkIndex := uint64(position / ChunkSize)
		chunkStart := int64(chunkIndex) * ChunkSize

		var plain []byte
		var err error
		if sequential {
			plain, err = f.sequentialChunk(chunkIndex)
		} else {
			plain, err = f.decryptChunk(chunkIndex, scratch)
			scratch = plain
		}
		if err != nil {
			return written, err
		}

		within := int(position - chunkStart)
		take := len(plain) - within
		if remaining := limit - written; take > remaining {
			take = remaining
		}
		copy(p[written:written+take], plain[within:within+take])
		written += take
	}
	if written < requested {
		return written, io.EOF
	}
	return written, nil
}

func (f *File) sequentialChunk(index uint64) ([]byte, error) {
	if f.cacheValid && f.cachedChunk == index {
		return f.cachedPlain, nil
	}
	plain, err := f.decryptChunk(index, f.cachedPlain)
	if err != nil {
		f.cacheValid = false
		return nil, err
	}
	f.cachedChunk = index
	f.cachedPlain = plain
	f.cacheValid = true
	return f.cachedPlain, nil
}

func (f *File) decryptChunk(index uint64, buffer []byte) ([]byte, error) {
	plainLength := f.chunkPlainLength(index)
	cipherLength := plainLength + int64(f.aead.Overhead())
	cipherOffset := int64(headerSize) + int64(index)*(ChunkSize+int64(f.aead.Overhead()))

	if cap(buffer) < int(cipherLength) {
		buffer = make([]byte, int(cipherLength))
	} else {
		buffer = buffer[:int(cipherLength)]
	}
	if _, err := f.file.ReadAt(buffer, cipherOffset); err != nil {
		return nil, fmt.Errorf("%w: read ciphertext: %v", ErrInvalidFormat, err)
	}
	plain, err := f.aead.Open(buffer[:0], nonce(f.prefix, index), buffer, chunkAAD(f.core, index))
	if err != nil {
		return nil, ErrAuthentication
	}
	return plain, nil
}

func (f *File) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = f.offset + offset
	case io.SeekEnd:
		next = f.size + offset
	default:
		return f.offset, errors.New("encrypted file: invalid whence")
	}
	if next < 0 {
		return f.offset, errors.New("encrypted file: negative position")
	}
	f.offset = next
	return next, nil
}

func (f *File) Close() error       { return f.file.Close() }
func (f *File) Size() int64        { return f.size }
func (f *File) ModTime() time.Time { return f.modTime }

func (f *File) chunkPlainLength(index uint64) int64 {
	start := int64(index) * ChunkSize
	remaining := f.size - start
	if remaining > ChunkSize {
		return ChunkSize
	}
	return remaining
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != chacha20poly1305.KeySize {
		return nil, ErrInvalidKey
	}
	return chacha20poly1305.NewX(key)
}

func newHeaderCore(plaintextSize int64) ([]byte, [noncePrefixSize]byte, error) {
	var prefix [noncePrefixSize]byte
	if _, err := rand.Read(prefix[:]); err != nil {
		return nil, prefix, err
	}
	core := make([]byte, headerCoreSize)
	copy(core[:8], magic)
	core[8] = version
	binary.LittleEndian.PutUint64(core[16:24], uint64(plaintextSize))
	copy(core[24:40], prefix[:])
	return core, prefix, nil
}

func nonce(prefix [noncePrefixSize]byte, index uint64) []byte {
	out := make([]byte, chacha20poly1305.NonceSizeX)
	copy(out[:noncePrefixSize], prefix[:])
	binary.LittleEndian.PutUint64(out[noncePrefixSize:], index)
	return out
}

func chunkAAD(core []byte, index uint64) []byte {
	aad := make([]byte, len(core)+8)
	copy(aad, core)
	binary.LittleEndian.PutUint64(aad[len(core):], index)
	return aad
}

func chunkCount(size int64) uint64 {
	if size <= 0 {
		return 0
	}
	chunks := uint64(size / ChunkSize)
	if size%ChunkSize != 0 {
		chunks++
	}
	return chunks
}
