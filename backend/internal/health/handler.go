package health

import (
	"net/http"
	"time"

	"relay/internal/platform/database"
	"relay/internal/platform/httpserver"
)

// StatusResponse is the body of /api/status, shown on the React status page.
type StatusResponse struct {
	API      string             `json:"api"`
	Postgres DependencyStatus   `json:"postgres"`
	DBPool   database.PoolStats `json:"db_pool"`
	Version  string             `json:"version"`
	UptimeS  int64              `json:"uptime_s"`
}

// Options are the facts the handler reports but doesn't own.
type Options struct {
	Version      string
	StartedAt    time.Time
	PoolStats    func() database.PoolStats // live pool numbers (note 11)
	ShuttingDown func() bool               // true once graceful shutdown starts (note 12)
}

// Handler serves the health endpoints.
type Handler struct {
	checker *Checker
	opts    Options
}

func NewHandler(checker *Checker, opts Options) *Handler {
	return &Handler{checker: checker, opts: opts}
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
	httpserver.WriteJSON(w, r, http.StatusOK, map[string]string{"status": "alive"})
}

// readyz answers "can this server handle requests right now?".
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	switch {
	case h.opts.ShuttingDown():
		httpserver.WriteJSON(w, r, http.StatusServiceUnavailable, map[string]string{"status": "shutting_down"})
	case h.checker.CheckPostgres(r.Context()).Status != StatusOK:
		httpserver.WriteJSON(w, r, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
	default:
		httpserver.WriteJSON(w, r, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// status is the friendly, detailed version of readyz for the UI.
func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	pg := h.checker.CheckPostgres(r.Context())
	resp := StatusResponse{
		API:      StatusOK, // we're answering, so the API is up
		Postgres: pg,
		DBPool:   h.opts.PoolStats(),
		Version:  h.opts.Version,
		UptimeS:  int64(time.Since(h.opts.StartedAt).Seconds()),
	}
	code := http.StatusOK
	if pg.Status != StatusOK {
		code = http.StatusServiceUnavailable
	}
	httpserver.WriteJSON(w, r, code, resp)
}
