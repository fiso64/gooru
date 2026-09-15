package serve

import (
	"net/http"
	"time"
)

// requestWriteTimeoutMiddleware turns http.Server.WriteTimeout into a per-write
// idle deadline. The server-level timeout is absolute from the end of request
// header parsing, which can otherwise terminate legitimate long-running media
// responses even while they are making steady progress.
func requestWriteTimeoutMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	if timeout <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&refreshingWriteDeadlineResponseWriter{
			ResponseWriter: w,
			timeout:        timeout,
		}, r)
	})
}

type refreshingWriteDeadlineResponseWriter struct {
	http.ResponseWriter
	timeout time.Duration
}

func (w *refreshingWriteDeadlineResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *refreshingWriteDeadlineResponseWriter) refresh() {
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Now().Add(w.timeout))
}

func (w *refreshingWriteDeadlineResponseWriter) WriteHeader(statusCode int) {
	w.refresh()
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *refreshingWriteDeadlineResponseWriter) Write(body []byte) (int, error) {
	w.refresh()
	return w.ResponseWriter.Write(body)
}
