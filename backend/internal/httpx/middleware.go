package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Recover turns a panic in a handler into a 500 instead of a dropped connection,
// and logs the stack so the bug can be found.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler { // deliberate abort: let net/http handle it
					panic(v)
				}
				slog.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				WriteJSON(w, http.StatusInternalServerError,
					map[string]*Error{"error": NewError(http.StatusInternalServerError, "internal", "something went wrong")})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Log writes one line per request: method, path, status, duration.
func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		// Health probes run every few seconds; logging them would drown everything else.
		if r.URL.Path == "/livez" || r.URL.Path == "/readyz" {
			return
		}
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", sw.status,
			"duration_ms", float64(time.Since(start).Microseconds())/1000)
	})
}

// statusWriter remembers the status code. Unwrap lets http.ResponseController
// (and the WebSocket library) reach the real writer to hijack the connection.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
