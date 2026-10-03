package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorBody {
	t.Helper()
	var env struct {
		Error ErrorBody `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("body is not our error shape: %v", err)
	}
	return env.Error
}

func TestWriteError_UsesTheSharedShape(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	rec.Header().Set(RequestIDHeader, "req123")

	WriteError(rec, req, http.StatusBadRequest, CodeValidationFailed, "Some fields are invalid.",
		map[string]string{"body": "must be 1-1000 characters"})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	got := decodeError(t, rec)
	if got.Code != CodeValidationFailed || got.Message != "Some fields are invalid." ||
		got.Fields["body"] != "must be 1-1000 characters" || got.RequestID != "req123" {
		t.Fatalf("unexpected error body: %+v", got)
	}
}

func TestWriteError_NoFields_OmitsFieldsKey(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusNotFound, CodeNotFound, "Not found.", nil)

	if strings.Contains(rec.Body.String(), `"fields"`) {
		t.Fatalf("fields should be omitted when empty: %s", rec.Body.String())
	}
}

func TestDecodeJSON_Valid(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"body":"hi"}`))
	var dst struct {
		Body string `json:"body"`
	}
	if err := DecodeJSON(httptest.NewRecorder(), req, &dst); err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}
	if dst.Body != "hi" {
		t.Fatalf("Body = %q", dst.Body)
	}
}

func TestDecodeJSON_Rejects(t *testing.T) {
	tests := map[string]string{
		"malformed":     `{"body":`,
		"unknown field": `{"body":"hi","admin":true}`,
		"two objects":   `{"body":"a"}{"body":"b"}`,
		"empty":         ``,
		"too large":     `{"body":"` + strings.Repeat("x", maxBodyBytes) + `"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			var dst struct {
				Body string `json:"body"`
			}
			if err := DecodeJSON(httptest.NewRecorder(), req, &dst); err == nil {
				t.Fatal("DecodeJSON() = nil, want error")
			}
		})
	}
}

func TestNotFound_UsesTheSharedShape(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
	if got := decodeError(t, rec); got.Code != CodeNotFound {
		t.Fatalf("code = %q, want %q", got.Code, CodeNotFound)
	}
}

func TestNewMux_UnknownRoute_GetsOurJSON404(t *testing.T) {
	mux := NewMux()
	mux.HandleFunc("GET /known", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })

	if rec := serve(mux, http.MethodGet, "/known"); rec.Code != http.StatusNoContent {
		t.Fatalf("known route code = %d, want 204", rec.Code)
	}
	rec := serve(mux, http.MethodGet, "/api/does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown route code = %d, want 404", rec.Code)
	}
	if got := decodeError(t, rec); got.Code != CodeNotFound {
		t.Fatalf("error code = %q, want %q", got.Code, CodeNotFound)
	}
}
