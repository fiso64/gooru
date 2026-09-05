package filesource

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gooru.local/internal/encryptedfile"
)

// Reader is the random-access plaintext view feature code receives for a
// logical file. Storage adapters may back it with an ordinary file or an
// authenticated encrypted container.
type Reader interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

// Source is an opened logical file together with the metadata that describes
// its plaintext contents rather than its physical storage representation.
type Source struct {
	Reader
	size    int64
	modTime time.Time
}

func (s *Source) Size() int64        { return s.size }
func (s *Source) ModTime() time.Time { return s.modTime }

// Metadata describes a logical file without requiring callers to open it.
type Metadata struct {
	Size    int64
	ModTime time.Time
}

// Resolver owns the policy for translating a tracked filesystem location into
// its logical plaintext source. With an encryption key configured, encrypted
// tracked files are authenticated/decrypted regardless of whether they remain
// under a current managed root. Plaintext paths outside protected roots remain
// ordinary filesystem files, while plaintext paths inside protected roots fail
// closed so callers cannot accidentally consume an unencrypted managed file.
type Resolver struct {
	encryptionKey  []byte
	protectedRoots []string
}

// NewFilesystem returns a resolver for ordinary plaintext libraries.
func NewFilesystem() *Resolver { return &Resolver{} }

// NewProtected returns a resolver that can authenticate encrypted tracked files
// and requires encryption beneath protectedRoots. Roots are canonicalized once
// so the common Open path does not repeatedly resolve them.
func NewProtected(encryptionKey []byte, protectedRoots []string) (*Resolver, error) {
	if len(encryptionKey) != 32 {
		return nil, encryptedfile.ErrInvalidKey
	}
	roots := make([]string, 0, len(protectedRoots))
	seen := make(map[string]struct{}, len(protectedRoots))
	for _, root := range protectedRoots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("resolve protected file root: %w", err)
		}
		abs = filepath.Clean(abs)
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		roots = append(roots, abs)
	}
	return &Resolver{
		encryptionKey:  append([]byte(nil), encryptionKey...),
		protectedRoots: roots,
	}, nil
}

// IsProtectedPath reports whether path is inside one of the configured managed
// roots. It is purely lexical after absolute-path normalization; upload-target
// validation is responsible for rejecting unsafe root configuration.
func (r *Resolver) IsProtectedPath(path string) bool {
	if r == nil || len(r.encryptionKey) == 0 || len(r.protectedRoots) == 0 {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	abs = filepath.Clean(abs)
	for _, root := range r.protectedRoots {
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !startsWithParent(rel)) {
			return true
		}
	}
	return false
}

func startsWithParent(rel string) bool {
	return rel == ".." || len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator)
}

// Metadata returns logical plaintext metadata. Ordinary filesystem mode keeps
// this path stat-only, which is important for relink scans over large libraries.
// Protected mode authenticates encrypted containers to recover their plaintext
// size and fails closed for unexpected plaintext beneath managed roots.
func (r *Resolver) Metadata(path string) (Metadata, error) {
	if r != nil && len(r.encryptionKey) > 0 {
		encrypted, err := encryptedfile.IsEncryptedFile(path)
		if err != nil {
			return Metadata{}, err
		}
		if encrypted {
			file, err := encryptedfile.Open(path, r.encryptionKey)
			if err != nil {
				return Metadata{}, err
			}
			defer file.Close()
			return Metadata{Size: file.Size(), ModTime: file.ModTime()}, nil
		}
		if r.IsProtectedPath(path) {
			return Metadata{}, fmt.Errorf("protected managed file is not encrypted: %w", encryptedfile.ErrInvalidFormat)
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return Metadata{}, err
	}
	if info.IsDir() {
		return Metadata{}, fmt.Errorf("logical file source is a directory")
	}
	if !info.Mode().IsRegular() {
		return Metadata{}, errors.New("logical file source is not a regular file")
	}
	return Metadata{Size: info.Size(), ModTime: info.ModTime()}, nil
}

// Open resolves path into its logical plaintext random-access source.
func (r *Resolver) Open(path string) (*Source, error) {
	if r != nil && len(r.encryptionKey) > 0 {
		encrypted, err := encryptedfile.IsEncryptedFile(path)
		if err != nil {
			return nil, err
		}
		if encrypted {
			file, err := encryptedfile.Open(path, r.encryptionKey)
			if err != nil {
				return nil, err
			}
			return &Source{Reader: file, size: file.Size(), modTime: file.ModTime()}, nil
		}
		if r.IsProtectedPath(path) {
			return nil, fmt.Errorf("protected managed file is not encrypted: %w", encryptedfile.ErrInvalidFormat)
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, fmt.Errorf("logical file source is a directory")
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("logical file source is not a regular file")
	}
	return &Source{Reader: file, size: info.Size(), modTime: info.ModTime()}, nil
}
