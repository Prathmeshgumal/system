package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"relay/internal/metrics"
)

// RequestIDHeader carries each request's ID back to the client. Error bodies
// carry it too, so "it broke, here's the ID" finds the exact log line.
const RequestIDHeader = "X-Request-ID"

// Info is what we learn about a request on its way through the layers. Log
// creates it; inner layers fill it in (the route pattern, who's calling); Log
// reads it back when the request is done.
type Info struct {
	ID     string
	Route  string // mux pattern, e.g. "/api/chats/{id}" — never the raw path
	UserID int64
}

type infoKey struct{}

// InfoFrom returns the request's Info, or nil outside of Log.
func InfoFrom(ctx context.Context) *Info {
	i, _ := ctx.Value(infoKey{}).(*Info)
	return i
}

// Logger returns slog's default logger tagged with the request ID.
func Logger(ctx context.Context) *slog.Logger {
	if i := InfoFrom(ctx); i != nil {
		return slog.Default().With("request_id", i.ID)
	}
	return slog.Default()
}

// Log is the outermost middleware. For every request it:
//   - gives it a fresh ID (X-Request-ID); IDs sent by clients are ignored, so
//     nobody can plant fake IDs in our logs
//   - writes one log line when it's done: method, route, status, duration, user
//   - records the same in Prometheus (metrics.ObserveHTTP)
func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &Info{ID: newRequestID()}
		w.Header().Set(RequestIDHeader, info.ID)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), infoKey{}, info)))

		route := info.Route
		if route == "" {
			route = "unmatched"
		}
		d := time.Since(start)
		metrics.ObserveHTTP(r.Method, route, sw.status, d)

		// Health probes run every few seconds; logging them would drown everything else.
		if route == "/livez" || route == "/readyz" {
			return
		}
		attrs := []any{"request_id", info.ID, "method", r.Method, "route", route, "path", r.URL.Path,
			"status", sw.status, "duration_ms", float64(d.Microseconds()) / 1000}
		if info.UserID != 0 {
			attrs = append(attrs, "user_id", info.UserID)
		}
		slog.Info("request", attrs...)
	})
}

// Route records which mux pattern handled the request. Wrap every mux with it:
// with nested muxes the innermost (most specific) pattern wins, because it
// returns first and outer ones don't overwrite it.
func Route(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r)
		// ServeMux stores the matched pattern on the request it was given.
		if i := InfoFrom(r.Context()); i != nil && i.Route == "" && r.Pattern != "" && !strings.HasSuffix(r.Pattern, "/") {
			_, path, found := strings.Cut(r.Pattern, " ") // "GET /api/x" → "/api/x"
			if !found {
				path = r.Pattern
			}
			i.Route = path
		}
	})
}

// Recover turns a panic in a handler into a 500 instead of a dropped connection,
// and logs the stack so the bug can be found.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler { // deliberate abort: let net/http handle it
					panic(v)
				}
				Logger(r.Context()).Error("panic", "method", r.Method, "path", r.URL.Path, "panic", v,
					"stack", string(debug.Stack()))
				WriteError(w, r, NewError(http.StatusInternalServerError, "internal", "something went wrong"))
			}
		}()
		next.ServeHTTP(w, r)
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

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
