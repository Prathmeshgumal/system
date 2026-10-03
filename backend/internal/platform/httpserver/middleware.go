package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"go.uber.org/zap"

	"relay/internal/platform/logging"
)

// RequestIDHeader carries the request ID back to the client.
const RequestIDHeader = "X-Request-ID"

// Middleware wraps a handler with behaviour that runs before and/or after it.
type Middleware func(http.Handler) http.Handler

// Chain wraps h so that mws run in the order given: Chain(h, a, b) runs
// a → b → h on the way in, and h → b → a on the way out.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// Recover turns a panic in any handler into a 500 response and a log line,
// instead of losing the response. It must be the outermost middleware.
func Recover(log *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				if p == http.ErrAbortHandler {
					panic(p) // the standard "abort this response" signal; let net/http handle it
				}
				log.Error("panic recovered",
					zap.Any("panic", p),
					zap.String("request_id", w.Header().Get(RequestIDHeader)),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Stack("stack"),
				)
				WriteError(w, r, http.StatusInternalServerError, CodeInternal, "Something went wrong.", nil)
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestID gives every request a fresh ID: set as the X-Request-ID response
// header and attached to the request's logger. Client-supplied IDs are ignored
// so nobody can inject fake IDs into our logs.
func RequestID(base *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newRequestID()
			w.Header().Set(RequestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			ctx = logging.WithLogger(ctx, base.With(zap.String("request_id", id)))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestLog writes one line per request after it finishes: method, path,
// status and duration. It uses the logger from RequestID, so it must run inside it.
// quietPaths (e.g. health probes, hit every few seconds) are logged at debug
// level so they don't bury real traffic in production.
func RequestLog(quietPaths ...string) Middleware {
	quiet := make(map[string]bool, len(quietPaths))
	for _, p := range quietPaths {
		quiet[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			level := zap.InfoLevel
			if quiet[r.URL.Path] {
				level = zap.DebugLevel
			}
			logging.FromContext(r.Context()).Log(level, "request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", sw.status),
				zap.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			)
		})
	}
}

// statusWriter remembers the status code a handler wrote — Go's
// ResponseWriter doesn't let you read it back. If the handler never calls
// WriteHeader, Go sends 200, which is our starting value.
type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (sw *statusWriter) WriteHeader(code int) {
	if !sw.wroteHeader {
		sw.status, sw.wroteHeader = code, true
	}
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	sw.wroteHeader = true
	return sw.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer (needed later for
// WebSockets and streaming).
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }

type requestIDKey struct{}

// requestID returns the current request's ID: from the context if the
// RequestID middleware ran, otherwise from the response header it set
// (Recover sits outside RequestID, so it only sees the header).
func requestID(w http.ResponseWriter, r *http.Request) string {
	if id, ok := r.Context().Value(requestIDKey{}).(string); ok {
		return id
	}
	return w.Header().Get(RequestIDHeader)
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b) // crypto/rand never fails on supported platforms
	return hex.EncodeToString(b)
}
