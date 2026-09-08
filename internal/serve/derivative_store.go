package serve

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gooru.local/internal/encryptedfile"
)

const encryptedDerivativeNamespace = "protected-v1"

type derivativeGenerator func(io.Writer) error

type derivativeArtifact struct {
	Reader       io.ReadSeeker
	Name         string
	ModTime      time.Time
	CacheStatus  string
	CacheControl string
	close        func() error
}

func (a *derivativeArtifact) Close() error {
	if a == nil || a.close == nil {
		return nil
	}
	return a.close()
}

type derivativeStore interface {
	GetOrGenerate(relativePath string, generate derivativeGenerator) (*derivativeArtifact, error)
}

type memoryDerivativeStore struct{}

func (memoryDerivativeStore) GetOrGenerate(relativePath string, generate derivativeGenerator) (*derivativeArtifact, error) {
	var out bytes.Buffer
	if err := generate(&out); err != nil {
		return nil, err
	}
	return &derivativeArtifact{
		Reader:       bytes.NewReader(out.Bytes()),
		Name:         filepath.Base(relativePath),
		CacheStatus:  "bypass",
		CacheControl: "private, no-store",
	}, nil
}

type derivativePathLocker struct {
	mu    sync.Mutex
	locks map[string]*derivativePathLock
}

type derivativePathLock struct {
	mu   sync.Mutex
	refs int
}

func (l *derivativePathLocker) lock(path string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = make(map[string]*derivativePathLock)
	}
	lock := l.locks[path]
	if lock == nil {
		lock = &derivativePathLock{}
		l.locks[path] = lock
	}
	lock.refs++
	l.mu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		l.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(l.locks, path)
		}
		l.mu.Unlock()
	}
}

type persistentDerivativeStore struct {
	root string
	derivativePathLocker
}

func newPersistentDerivativeStore(root string) *persistentDerivativeStore {
	return &persistentDerivativeStore{root: root}
}

func (s *persistentDerivativeStore) GetOrGenerate(relativePath string, generate derivativeGenerator) (*derivativeArtifact, error) {
	path, err := derivativePath(s.root, relativePath)
	if err != nil {
		return nil, err
	}
	artifact, err := openPersistentDerivative(path, "hit")
	if err == nil {
		return artifact, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	unlock := s.lock(path)
	defer unlock()
	artifact, err = openPersistentDerivative(path, "hit")
	if err == nil {
		return artifact, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gooru-derivative-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	genErr := generate(tmp)
	closeErr := tmp.Close()
	if genErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if genErr != nil {
			return nil, genErr
		}
		return nil, closeErr
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	return openPersistentDerivative(path, "miss")
}

type encryptedDerivativeStore struct {
	root string
	key  []byte
	derivativePathLocker
}

func newEncryptedDerivativeStore(root string, key []byte) *encryptedDerivativeStore {
	return &encryptedDerivativeStore{
		root: filepath.Join(root, encryptedDerivativeNamespace),
		key:  append([]byte(nil), key...),
	}
}

func (s *encryptedDerivativeStore) GetOrGenerate(relativePath string, generate derivativeGenerator) (*derivativeArtifact, error) {
	path, err := derivativePath(s.root, relativePath)
	if err != nil {
		return nil, err
	}
	if artifact, err := s.open(path, "hit"); err == nil {
		return artifact, nil
	}

	unlock := s.lock(path)
	defer unlock()
	artifact, err := s.open(path, "hit")
	if err == nil {
		return artifact, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		// Derivatives are disposable. If an existing protected cache entry cannot
		// be authenticated/read (for example after corruption), remove it while
		// holding the per-path lock and regenerate rather than permanently failing
		// every request for that derivative.
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return nil, fmt.Errorf("remove unreadable protected derivative: %w", removeErr)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}

	// Derivatives are bounded outputs. Buffer plaintext in memory so the encrypted
	// file format can authenticate its declared size without ever materializing a
	// plaintext cache file on disk.
	var plain bytes.Buffer
	if err := generate(&plain); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gooru-derivative-encrypted-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	encryptErr := encryptedfile.Encrypt(tmp, bytes.NewReader(plain.Bytes()), int64(plain.Len()), s.key)
	closeErr := tmp.Close()
	if encryptErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		if encryptErr != nil {
			return nil, encryptErr
		}
		return nil, closeErr
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return nil, err
	}
	return s.open(path, "miss")
}

func (s *encryptedDerivativeStore) open(path, status string) (*derivativeArtifact, error) {
	file, err := encryptedfile.Open(path, s.key)
	if err != nil {
		return nil, err
	}
	return &derivativeArtifact{
		Reader:       file,
		Name:         filepath.Base(path),
		ModTime:      file.ModTime().Truncate(time.Second),
		CacheStatus:  status,
		CacheControl: "private, no-store",
		close:        file.Close,
	}, nil
}

func derivativePath(root, relativePath string) (string, error) {
	clean := filepath.Clean(relativePath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || len(clean) > 3 && clean[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("invalid derivative cache path")
	}
	return filepath.Join(root, clean), nil
}

func openPersistentDerivative(path, status string) (*derivativeArtifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &derivativeArtifact{
		Reader:       file,
		Name:         info.Name(),
		ModTime:      info.ModTime().Truncate(time.Second),
		CacheStatus:  status,
		CacheControl: "public, max-age=31536000, immutable",
		close:        file.Close,
	}, nil
}
