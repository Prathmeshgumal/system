package health

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// StatusResponse is the body of /api/status, shown on the React status page.
type StatusResponse struct {
	API      string           `json:"api"`
	Postgres DependencyStatus `json:"postgres"`
	Version  string           `json:"version"`
	UptimeS  int64            `json:"uptime_s"`
}

// Handler serves the health endpoints.
type Handler struct {
	checker   *Checker
	version   string
	startedAt time.Time
}

func NewHandler(checker *Checker, version string, startedAt time.Time) *Handler {
	return &Handler{checker: checker, version: version, startedAt: startedAt}
}

// Register adds the health routes to mux.
//
//	/livez, /readyz — for infrastructure; the proxy does not expose them.
//	/api/status     — for the UI; reachable through the proxy.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /livez", h.livez)
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /api/status", h.status)
}

// livez answers "is the process alive?". It deliberately checks nothing else:
// if this handler runs at all, the answer is yes.
func (h *Handler) livez(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

// readyz answers "can this server handle requests right now?".
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	if h.checker.CheckPostgres(r.Context()).Status != StatusOK {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// status is the friendly, detailed version of readyz for the UI.
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	pg := h.checker.CheckPostgres(r.Context())
	resp := StatusResponse{
		API:      StatusOK, // we're answering, so the API is up
		Postgres: pg,
		Version:  h.version,
		UptimeS:  int64(time.Since(h.startedAt).Seconds()),
	}
	code := http.StatusOK
	if pg.Status != StatusOK {
		code = http.StatusServiceUnavailable
	}
	writeJSON(w, code, resp)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json response", "err", err)
	}
}
