package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"relay/internal/platform/database"
)

// newTestMux wires the health handlers to a fake database that is up or down.
func newTestMux(dbUp bool) *http.ServeMux {
	return newTestMuxWith(dbUp, false)
}

func newTestMuxWith(dbUp, shuttingDown bool) *http.ServeMux {
	var db fakePinger
	if !dbUp {
		db.err = errors.New("connection refused")
	}
	h := NewHandler(NewChecker(db, time.Second), Options{
		Version:      "test-version",
		StartedAt:    time.Now(),
		PoolStats:    func() database.PoolStats { return database.PoolStats{InUse: 1, Idle: 2, Total: 3, Max: 10, Waited: 4} },
		ShuttingDown: func() bool { return shuttingDown },
	})
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func get(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestLivez_DBUp_Returns200(t *testing.T) {
	rec := get(t, newTestMux(true), "/livez")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

// The key rule from Note 2: liveness must NOT depend on the database,
// otherwise a DB blip makes the orchestrator restart every server.
func TestLivez_DBDown_StillReturns200(t *testing.T) {
	rec := get(t, newTestMux(false), "/livez")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 — liveness must ignore the DB", rec.Code)
	}
}

func TestReadyz_DBUp_Returns200(t *testing.T) {
	rec := get(t, newTestMux(true), "/readyz")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}

func TestReadyz_DBDown_Returns503(t *testing.T) {
	rec := get(t, newTestMux(false), "/readyz")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
}

func TestStatus_DBUp_Returns200WithDetails(t *testing.T) {
	rec := get(t, newTestMux(true), "/api/status")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.API != StatusOK {
		t.Errorf("api = %q, want %q", body.API, StatusOK)
	}
	if body.Postgres.Status != StatusOK {
		t.Errorf("postgres.status = %q, want %q", body.Postgres.Status, StatusOK)
	}
	if body.Version != "test-version" {
		t.Errorf("version = %q, want %q", body.Version, "test-version")
	}
	if body.UptimeS < 0 {
		t.Errorf("uptime_s = %d, want >= 0", body.UptimeS)
	}
}

// When the DB is down the API itself is still answering, so "api" stays ok
// and only the postgres entry reports down.
func TestStatus_DBDown_Returns503WithPostgresDown(t *testing.T) {
	rec := get(t, newTestMux(false), "/api/status")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.API != StatusOK {
		t.Errorf("api = %q, want %q", body.API, StatusOK)
	}
	if body.Postgres.Status != StatusDown {
		t.Errorf("postgres.status = %q, want %q", body.Postgres.Status, StatusDown)
	}
}

func TestHealthEndpoints_RejectNonGET(t *testing.T) {
	mux := newTestMux(true)
	for _, path := range []string{"/livez", "/readyz", "/api/status"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s: code = %d, want 405", path, rec.Code)
		}
	}
}

func TestStatus_IncludesPoolStats(t *testing.T) {
	rec := get(t, newTestMux(true), "/api/status")

	var body StatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	want := database.PoolStats{InUse: 1, Idle: 2, Total: 3, Max: 10, Waited: 4}
	if body.DBPool != want {
		t.Fatalf("db_pool = %+v, want %+v", body.DBPool, want)
	}
}

// During the shutdown drain (note 12) readiness fails with a distinct status,
// so the logs show *why* — and liveness stays OK, so nobody restarts us.
func TestReadyz_ShuttingDown_Returns503ShuttingDown(t *testing.T) {
	rec := get(t, newTestMuxWith(true, true), "/readyz")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	var body map[string]string
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body["status"] != "shutting_down" {
		t.Fatalf("status = %q, want shutting_down", body["status"])
	}
}

func TestLivez_ShuttingDown_StillReturns200(t *testing.T) {
	if rec := get(t, newTestMuxWith(true, true), "/livez"); rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
}
