package users

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"relay/internal/httpx"
)

type Handler struct {
	store *Store
}

func NewHandler(s *Store) *Handler { return &Handler{store: s} }

// Register adds the routes. All of them need a logged-in user, so mux here is
// the authenticated mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/me", httpx.Handle(h.me))
	mux.Handle("PATCH /api/me", httpx.Handle(h.updateMe))
	mux.Handle("GET /api/users", httpx.Handle(h.search))
	mux.Handle("GET /api/users/{id}", httpx.Handle(h.get))
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	u, err := h.store.GetByID(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, u)
	return nil
}

func (h *Handler) updateMe(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Username    *string `json:"username"`
		DisplayName *string `json:"display_name"`
		Bio         *string `json:"bio"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	fields := map[string]string{}
	var upd ProfileUpdate
	if in.Username != nil {
		v, msg := NormalizeUsername(*in.Username)
		upd.Username = &v
		if msg != "" {
			fields["username"] = msg
		}
	}
	if in.DisplayName != nil {
		v, msg := NormalizeDisplayName(*in.DisplayName)
		upd.DisplayName = &v
		if msg != "" {
			fields["display_name"] = msg
		}
	}
	if in.Bio != nil {
		v, msg := NormalizeBio(*in.Bio)
		upd.Bio = &v
		if msg != "" {
			fields["bio"] = msg
		}
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	u, err := h.store.Update(r.Context(), httpx.UserID(r.Context()), upd)
	if errors.Is(err, ErrUsernameTaken) {
		return &httpx.Error{Status: http.StatusConflict, Code: "username_taken", Message: "that username is taken",
			Fields: map[string]string{"username": "already taken"}}
	}
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, u)
	return nil
}

// search: GET /api/users?q=al → up to 20 users whose username starts with "al".
func (h *Handler) search(w http.ResponseWriter, r *http.Request) error {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	out := []Profile{}
	if q != "" && len(q) <= 32 {
		var err error
		out, err = h.store.Search(r.Context(), q, httpx.UserID(r.Context()), 20)
		if err != nil {
			return err
		}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string][]Profile{"users": out})
	return nil
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return httpx.NotFound("user not found")
	}
	u, err := h.store.GetByID(r.Context(), id)
	if errors.Is(err, ErrNotFound) {
		return httpx.NotFound("user not found")
	}
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, u.Profile) // public part only: no email
	return nil
}
