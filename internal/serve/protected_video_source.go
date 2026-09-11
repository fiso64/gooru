package serve

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// logicalVideoSeekablePath exposes one already-open logical video source through
// a short-lived, tokenized loopback HTTP endpoint. ffmpeg/ffprobe receive the
// same seek/range semantics they get from an ordinary clear-mode pathname while
// storage remains abstract: protected callers can back the source with the
// authenticated encrypted random-access reader without plaintext materialization
// or whole-video buffering.
func logicalVideoSeekablePath(name string, src io.ReaderAt, size int64) (string, func(), bool, error) {
	if src == nil || size < 0 {
		return "", nil, false, nil
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
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}

		// Media backends may stop reading an initial response and immediately open
		// a second Range request when seeking. A shared seek cursor protected for
		// the lifetime of ServeContent deadlocks that pattern: the abandoned first
		// response can block while writing and keep the Range request from seeking.
		// ReaderAt is safe to project into an independent SectionReader per request,
		// preserving bounded protected-file random access without serializing whole
		// HTTP responses behind one cursor lock.
		reader := io.NewSectionReader(src, 0, size)
		http.ServeContent(w, r, "source"+ext, time.Time{}, reader)
	})
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	cleanup := func() { _ = server.Close() }
	return "http://" + listener.Addr().String() + path, cleanup, true, nil
}
