// Package httpserver holds the HTTP plumbing shared by every feature: JSON and
// error responses, middleware, and the server with its shutdown sequence.
// See knowledge-base/notes/phase-0-notes.md, notes 7, 12 and 13.
package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"relay/internal/platform/logging"
)

// Error codes are stable and machine-readable: the frontend branches on them,
// so they are never reworded. Messages are for humans and may change.
const (
	CodeInvalidJSON      = "invalid_json"
	CodeValidationFailed = "validation_failed"
	CodeNotFound         = "not_found"
	CodeUnavailable      = "unavailable"
	CodeInternal         = "internal"
)

// maxBodyBytes caps request bodies so a client can't make us read gigabytes.
const maxBodyBytes = 1 << 20 // 1 MiB

// ErrorBody is the shape of every error response:
//
//	{"error": {"code": "...", "message": "...", "fields": {...}, "request_id": "..."}}
type ErrorBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id"`
}

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logging.FromContext(r.Context()).Debug("write json response failed")
	}
}

// WriteError writes an error in the shared shape. fields may be nil.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, fields map[string]string) {
	WriteJSON(w, r, status, map[string]ErrorBody{"error": {
		Code:      code,
		Message:   message,
		Fields:    fields,
		RequestID: requestID(w, r),
	}})
}

// DecodeJSON reads exactly one JSON object from the request body into dst.
// It rejects bodies over 1 MiB, unknown fields and trailing data.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is empty")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if dec.More() {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// NotFound answers unknown routes in the shared error shape (instead of Go's
// plain-text "404 page not found"). Register it as the catch-all "/" route.
func NotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, CodeNotFound, "Not found.", nil)
	})
}

// NewMux returns a router whose unknown routes answer with our JSON 404.
// "/" is the catch-all pattern: it matches anything no other route does.
func NewMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", NotFound())
	return mux
}
