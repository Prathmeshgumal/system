package notes

import (
	"context"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"relay/internal/platform/httpserver"
	"relay/internal/platform/logging"
)

// service is what the handler needs. *Service satisfies it; tests use a fake.
type service interface {
	Create(ctx context.Context, body string) (Note, error)
	ListLatest(ctx context.Context) ([]Note, error)
}

// Handler translates HTTP to service calls and back. No rules, no SQL.
type Handler struct {
	svc service
}

func NewHandler(svc service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/notes", h.create)
	mux.HandleFunc("GET /api/notes", h.list)
}

type createRequest struct {
	Body string `json:"body"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := httpserver.DecodeJSON(w, r, &req); err != nil {
		httpserver.WriteError(w, r, http.StatusBadRequest, httpserver.CodeInvalidJSON, err.Error(), nil)
		return
	}

	note, err := h.svc.Create(r.Context(), req.Body)
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	httpserver.WriteJSON(w, r, http.StatusCreated, note)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	notes, err := h.svc.ListLatest(r.Context())
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if notes == nil {
		notes = []Note{} // always a JSON array, never null
	}
	httpserver.WriteJSON(w, r, http.StatusOK, map[string][]Note{"notes": notes})
}

// writeServiceError maps service errors to responses. This is the one place
// errors are logged (note 6); the client only gets a code and the request ID.
func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	var verr *ValidationError
	switch {
	case errors.As(err, &verr):
		httpserver.WriteError(w, r, http.StatusBadRequest, httpserver.CodeValidationFailed,
			"Some fields are invalid.", verr.Fields)
	case errors.Is(err, ErrUnavailable):
		logging.FromContext(r.Context()).Warn("database unavailable", zap.Error(err))
		httpserver.WriteError(w, r, http.StatusServiceUnavailable, httpserver.CodeUnavailable,
			"Service temporarily unavailable. Please try again.", nil)
	default:
		logging.FromContext(r.Context()).Error("unexpected error", zap.Error(err))
		httpserver.WriteError(w, r, http.StatusInternalServerError, httpserver.CodeInternal,
			"Something went wrong.", nil)
	}
}
