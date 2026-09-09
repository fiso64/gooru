package serve

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// logicalVideoSeekablePath exposes one already-open logical video source through
// a short-lived, tokenized loopback HTTP endpoint. ffmpeg/ffprobe receive the
// same seek/range semantics they get from an ordinary clear-mode pathname while
// storage remains abstract: protected callers can back the source with the
// authenticated encrypted random-access reader without plaintext materialization
// or whole-video buffering.
func logicalVideoSeekablePath(name string, src io.ReadSeeker) (string, func(), bool, error) {
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", nil, false, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return "", nil, false, nil
	}

	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = listener.Close()
		return "", nil, false, err
	}
	ext := strings.ToLower(filepath.Ext(name))
	path := "/" + hex.EncodeToString(tokenBytes) + "/source" + ext
	var sourceMu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		sourceMu.Lock()
		defer sourceMu.Unlock()
		if _, err := src.Seek(0, io.SeekStart); err != nil {
			http.Error(w, "source unavailable", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, "source"+ext, time.Time{}, src)
	})
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	cleanup := func() { _ = server.Close() }
	return "http://" + listener.Addr().String() + path, cleanup, true, nil
}
