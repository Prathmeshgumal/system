// Package httpx holds the small HTTP helpers every module shares:
// JSON in/out, one error format, and middleware.
//
// Error format (every non-2xx response from our handlers):
//
//	{"error": {"code": "invalid_input", "message": "...", "fields": {"email": "..."}}}
//
// `code` is for the frontend to branch on; `message` is for humans.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

// Error is an error that knows how to become an HTTP response.
type Error struct {
	Status  int               `json:"-"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message) }

func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func BadRequest(code, message string) *Error { return NewError(http.StatusBadRequest, code, message) }
func Unauthorized(message string) *Error {
	return NewError(http.StatusUnauthorized, "unauthorized", message)
}
func Forbidden(message string) *Error { return NewError(http.StatusForbidden, "forbidden", message) }
func NotFound(message string) *Error  { return NewError(http.StatusNotFound, "not_found", message) }
func Conflict(code, message string) *Error {
	return NewError(http.StatusConflict, code, message)
}

// Invalid reports per-field validation problems.
func Invalid(fields map[string]string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: "invalid_input", Message: "some fields are invalid", Fields: fields}
}

// HandlerFunc is an http.HandlerFunc that can return an error.
// Returning an error is the only way handlers report failures, so the
// error format can't drift between handlers.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts a HandlerFunc to http.Handler.
func Handle(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}

// StatusClientClosed is nginx's convention for "the client hung up before we
// answered". Nobody receives it; it only shows up in our request log.
const StatusClientClosed = 499

// WriteError writes err as JSON. Errors that aren't *Error are bugs or outages:
// they're logged in full, and the client gets a bare 500 (no internals leaked).
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		// The browser aborted (closed the tab, switched chats). Not our bug.
		w.WriteHeader(StatusClientClosed)
		return
	}
	var he *Error
	if !errors.As(err, &he) {
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		he = NewError(http.StatusInternalServerError, "internal", "something went wrong")
	}
	WriteJSON(w, he.Status, map[string]*Error{"error": he})
}

func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "err", err)
	}
}

// maxBody caps request bodies. Our largest legit body is a 4000-char message.
const maxBody = 1 << 20

// ReadJSON decodes exactly one JSON object into dst. Unknown fields are rejected,
// so a typo like {"emial": ...} fails loudly instead of being silently ignored.
func ReadJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return NewError(http.StatusRequestEntityTooLarge, "too_large", "request body is too large")
		}
		return BadRequest("bad_json", "request body is not valid JSON: "+err.Error())
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return BadRequest("bad_json", "request body must be a single JSON object")
	}
	return nil
}
