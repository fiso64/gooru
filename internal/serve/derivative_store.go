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
)

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

type persistentDerivativeStore struct {
	root  string
	mu    sync.Mutex
	locks map[string]*derivativePathLock
}

type derivativePathLock struct {
	mu   sync.Mutex
	refs int
}

func newPersistentDerivativeStore(root string) *persistentDerivativeStore {
	return &persistentDerivativeStore{root: root}
}

func (s *persistentDerivativeStore) GetOrGenerate(relativePath string, generate derivativeGenerator) (*derivativeArtifact, error) {
	path, err := s.path(relativePath)
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

func (s *persistentDerivativeStore) path(relativePath string) (string, error) {
	clean := filepath.Clean(relativePath)
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || len(clean) > 3 && clean[:3] == ".."+string(filepath.Separator) {
		return "", fmt.Errorf("invalid derivative cache path")
	}
	return filepath.Join(s.root, clean), nil
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

func (s *persistentDerivativeStore) lock(path string) func() {
	s.mu.Lock()
	if s.locks == nil {
		s.locks = make(map[string]*derivativePathLock)
	}
	lock := s.locks[path]
	if lock == nil {
		lock = &derivativePathLock{}
		s.locks[path] = lock
	}
	lock.refs++
	s.mu.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		s.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(s.locks, path)
		}
		s.mu.Unlock()
	}
}
