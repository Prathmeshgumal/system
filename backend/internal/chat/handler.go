package chat

import (
	"errors"
	"net/http"
	"strconv"

	"relay/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(s *Service) *Handler { return &Handler{svc: s} }

// Register adds the routes to the authenticated mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /api/chats", httpx.Handle(h.list))
	mux.Handle("POST /api/chats/direct", httpx.Handle(h.startDirect))
	mux.Handle("POST /api/chats/group", httpx.Handle(h.createGroup))
	mux.Handle("GET /api/chats/{id}", httpx.Handle(h.get))
	mux.Handle("PATCH /api/chats/{id}", httpx.Handle(h.rename))
	mux.Handle("POST /api/chats/{id}/members", httpx.Handle(h.addMembers))
	mux.Handle("DELETE /api/chats/{id}/members/{userId}", httpx.Handle(h.removeMember))
	mux.Handle("PATCH /api/chats/{id}/members/{userId}", httpx.Handle(h.setRole))
	mux.Handle("GET /api/chats/{id}/messages", httpx.Handle(h.history))
	mux.Handle("POST /api/chats/{id}/messages", httpx.Handle(h.send))
	mux.Handle("POST /api/chats/{id}/read", httpx.Handle(h.markRead))
	mux.Handle("PATCH /api/messages/{id}", httpx.Handle(h.edit))
	mux.Handle("DELETE /api/messages/{id}", httpx.Handle(h.delete))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	chats, err := h.svc.List(r.Context(), httpx.UserID(r.Context()))
	if err != nil {
		return toHTTP(err)
	}
	if chats == nil {
		chats = []ChatView{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chats": chats})
	return nil
}

func (h *Handler) startDirect(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		UserID int64 `json:"user_id"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	c, created, err := h.svc.StartDirect(r.Context(), httpx.UserID(r.Context()), in.UserID)
	if err != nil {
		return toHTTP(err)
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, c)
	return nil
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Name      string  `json:"name"`
		MemberIDs []int64 `json:"member_ids"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	c, err := h.svc.CreateGroup(r.Context(), httpx.UserID(r.Context()), in.Name, in.MemberIDs)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusCreated, c)
	return nil
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	c, err := h.svc.Get(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, c)
	return nil
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	c, err := h.svc.Rename(r.Context(), httpx.UserID(r.Context()), id, in.Name)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, c)
	return nil
}

func (h *Handler) addMembers(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		UserIDs []int64 `json:"user_ids"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	c, err := h.svc.AddMembers(r.Context(), httpx.UserID(r.Context()), id, in.UserIDs)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, c)
	return nil
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	userID, err := pathID(r, "userId")
	if err != nil {
		return err
	}
	if err := h.svc.RemoveMember(r.Context(), httpx.UserID(r.Context()), id, userID); err != nil {
		return toHTTP(err)
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) setRole(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	userID, err := pathID(r, "userId")
	if err != nil {
		return err
	}
	var in struct {
		Role string `json:"role"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	c, err := h.svc.SetRole(r.Context(), httpx.UserID(r.Context()), id, userID, in.Role)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, c)
	return nil
}

// history: ?before=<seq> scrolls back, ?after=<seq> catches up, neither = newest.
func (h *Handler) history(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	q := r.URL.Query()
	var p Page
	fields := map[string]string{}
	for name, dst := range map[string]*int64{"before": &p.Before, "after": &p.After} {
		if v := q.Get(name); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				fields[name] = "must be a non-negative integer"
			}
			*dst = n
		}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxPageSize {
			fields["limit"] = "must be 1–100"
		}
		p.Limit = n
	}
	if p.Before > 0 && p.After > 0 {
		fields["after"] = "use before or after, not both"
	}
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	msgs, more, err := h.svc.History(r.Context(), httpx.UserID(r.Context()), id, p)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"messages": msgs, "has_more": more})
	return nil
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Body     string `json:"body"`
		ClientID string `json:"client_id"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	m, created, err := h.svc.Send(r.Context(), httpx.UserID(r.Context()), id, in.Body, in.ClientID)
	if err != nil {
		return toHTTP(err)
	}
	status := http.StatusOK // a retry: here's the message you already sent
	if created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, m)
	return nil
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Seq int64 `json:"seq"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	seq, err := h.svc.MarkRead(r.Context(), httpx.UserID(r.Context()), id, in.Seq)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]int64{"last_read_seq": seq})
	return nil
}

func (h *Handler) edit(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	m, err := h.svc.Edit(r.Context(), httpx.UserID(r.Context()), id, in.Body)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, m)
	return nil
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	m, err := h.svc.Delete(r.Context(), httpx.UserID(r.Context()), id)
	if err != nil {
		return toHTTP(err)
	}
	httpx.WriteJSON(w, http.StatusOK, m)
	return nil
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || id <= 0 {
		return 0, httpx.NotFound("not found")
	}
	return id, nil
}

// toHTTP maps chat's errors onto HTTP. Unknown errors pass through and become 500s.
func toHTTP(err error) error {
	var invalid InvalidError
	var rule *RuleError
	switch {
	case errors.Is(err, ErrNotFound):
		return httpx.NotFound("not found")
	case errors.Is(err, ErrForbidden):
		return httpx.Forbidden("you can't do that in this chat")
	case errors.Is(err, ErrDeleted):
		return httpx.Conflict("message_deleted", "that message was deleted")
	case errors.As(err, &invalid):
		return httpx.Invalid(invalid)
	case errors.As(err, &rule):
		return httpx.BadRequest(rule.Code, rule.Message)
	}
	return err
}
