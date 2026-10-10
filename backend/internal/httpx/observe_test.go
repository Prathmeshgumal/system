package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func captureLogs(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// server mimics app.New: outer mux → auth-like middleware → inner mux.
func server() http.Handler {
	inner := http.NewServeMux()
	inner.Handle("GET /api/chats/{id}", Handle(func(w http.ResponseWriter, r *http.Request) error {
		return NotFound("no such chat")
	}))
	outer := http.NewServeMux()
	outer.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Route(inner).ServeHTTP(w, r.WithContext(WithUserID(r.Context(), 42))) // what RequireUser does
	}))
	return Log(Recover(Route(outer)))
}

func TestObserve_RequestIDRouteAndUserInLogsAndErrors(t *testing.T) {
	logs := captureLogs(t)
	rec := httptest.NewRecorder()
	server().ServeHTTP(rec, httptest.NewRequest("GET", "/api/chats/123", nil))

	id := rec.Header().Get("X-Request-ID")
	if len(id) != 16 {
		t.Fatalf("X-Request-ID = %q", id)
	}
	var body struct {
		Error struct {
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Error.RequestID != id {
		t.Errorf("error body request_id = %q, want %q", body.Error.RequestID, id)
	}

	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(logs.String())), &line); err != nil {
		t.Fatalf("log is not one JSON line: %q", logs)
	}
	if line["request_id"] != id || line["route"] != "/api/chats/{id}" || line["user_id"] != float64(42) || line["status"] != float64(404) {
		t.Errorf("log line = %v", line)
	}
}

func TestObserve_ClientIDsAreIgnored(t *testing.T) {
	captureLogs(t)
	req := httptest.NewRequest("GET", "/api/chats/1", nil)
	req.Header.Set("X-Request-ID", "forged-by-the-client")
	rec := httptest.NewRecorder()
	server().ServeHTTP(rec, req)
	if rec.Header().Get("X-Request-ID") == "forged-by-the-client" {
		t.Error("a client-supplied ID was trusted")
	}
}

func TestObserve_UnmatchedRoute(t *testing.T) {
	logs := captureLogs(t)
	Log(Route(http.NewServeMux())).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/nope/123", nil))
	if !strings.Contains(logs.String(), `"route":"unmatched"`) {
		t.Errorf("logs = %s", logs)
	}
}
