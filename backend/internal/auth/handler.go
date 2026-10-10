package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"relay/internal/httpx"
	"relay/internal/users"
)

const refreshCookie = "relay_refresh"

// UserStore is what auth needs from the users module.
type UserStore interface {
	Create(ctx context.Context, n users.NewUser) (users.User, error)
	GetByEmail(ctx context.Context, email string) (users.User, error)
	GetByID(ctx context.Context, id int64) (users.User, error)
}

type Handler struct {
	users        UserStore
	sessions     *Sessions
	tokens       *Tokens
	hasher       *Hasher
	cookieSecure bool
}

func NewHandler(u UserStore, s *Sessions, t *Tokens, h *Hasher, cookieSecure bool) *Handler {
	return &Handler{users: u, sessions: s, tokens: t, hasher: h, cookieSecure: cookieSecure}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("POST /api/auth/signup", httpx.Handle(h.signup))
	mux.Handle("POST /api/auth/login", httpx.Handle(h.login))
	mux.Handle("POST /api/auth/refresh", httpx.Handle(h.refresh))
	mux.Handle("POST /api/auth/logout", httpx.Handle(h.logout))
}

// authResponse is returned by signup, login and refresh.
type authResponse struct {
	AccessToken string     `json:"access_token"`
	ExpiresAt   time.Time  `json:"expires_at"`
	User        users.User `json:"user"`
}

func (h *Handler) signup(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email       string `json:"email"`
		Username    string `json:"username"`
		DisplayName string `json:"display_name"`
		Password    string `json:"password"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}

	fields := map[string]string{}
	email, msg := users.NormalizeEmail(in.Email)
	addField(fields, "email", msg)
	username, msg := users.NormalizeUsername(in.Username)
	addField(fields, "username", msg)
	name, msg := users.NormalizeDisplayName(in.DisplayName)
	addField(fields, "display_name", msg)
	addField(fields, "password", ValidatePassword(in.Password))
	if len(fields) > 0 {
		return httpx.Invalid(fields)
	}

	hash, err := h.hasher.Hash(in.Password)
	if err != nil {
		return err
	}
	u, err := h.users.Create(r.Context(), users.NewUser{
		Email: email, Username: username, DisplayName: name, PasswordHash: hash,
	})
	switch {
	case errors.Is(err, users.ErrEmailTaken):
		return &httpx.Error{Status: http.StatusConflict, Code: "email_taken", Message: "that email is already registered",
			Fields: map[string]string{"email": "already registered"}}
	case errors.Is(err, users.ErrUsernameTaken):
		return &httpx.Error{Status: http.StatusConflict, Code: "username_taken", Message: "that username is taken",
			Fields: map[string]string{"username": "already taken"}}
	case err != nil:
		return err
	}
	return h.startSession(w, r, u, http.StatusCreated)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	// Same answer for "no such email" and "wrong password", in the same time.
	bad := httpx.NewError(http.StatusUnauthorized, "invalid_credentials", "invalid email or password")

	email, msg := users.NormalizeEmail(in.Email)
	if msg != "" {
		h.hasher.VerifyNobody(in.Password)
		return bad
	}
	u, err := h.users.GetByEmail(r.Context(), email)
	if errors.Is(err, users.ErrNotFound) {
		h.hasher.VerifyNobody(in.Password)
		return bad
	}
	if err != nil {
		return err
	}
	if !h.hasher.Verify(u.PasswordHash, in.Password) {
		return bad
	}
	return h.startSession(w, r, u, http.StatusOK)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) error {
	if err := checkOrigin(r); err != nil {
		return err
	}
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		return httpx.Unauthorized("not logged in")
	}
	sess, tok, err := h.sessions.Rotate(r.Context(), c.Value)
	if errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrTokenReused) {
		h.clearCookie(w)
		return httpx.Unauthorized("session expired, please log in again")
	}
	if err != nil {
		return err
	}
	u, err := h.users.GetByID(r.Context(), sess.UserID)
	if err != nil {
		return err
	}
	return h.respond(w, u, sess, tok, http.StatusOK)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	if err := checkOrigin(r); err != nil {
		return err
	}
	if c, err := r.Cookie(refreshCookie); err == nil {
		if err := h.sessions.Revoke(r.Context(), c.Value); err != nil {
			return err
		}
	}
	h.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) startSession(w http.ResponseWriter, r *http.Request, u users.User, status int) error {
	sess, tok, err := h.sessions.Create(r.Context(), u.ID, r.UserAgent())
	if err != nil {
		return err
	}
	return h.respond(w, u, sess, tok, status)
}

func (h *Handler) respond(w http.ResponseWriter, u users.User, sess Session, refreshTok string, status int) error {
	access, exp, err := h.tokens.IssueAccess(u.ID, sess.ID)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    refreshTok,
		Path:     "/api/auth", // the browser sends it ONLY to refresh and logout
		Expires:  sess.ExpiresAt,
		HttpOnly: true,                    // JavaScript (and so XSS) can't read it
		Secure:   h.cookieSecure,          // HTTPS only, once we have HTTPS (Phase 2)
		SameSite: http.SameSiteStrictMode, // never sent on requests started by other sites (CSRF)
	})
	httpx.WriteJSON(w, status, authResponse{AccessToken: access, ExpiresAt: exp, User: u})
	return nil
}

func (h *Handler) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: refreshCookie, Path: "/api/auth", MaxAge: -1,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteStrictMode,
	})
}

// checkOrigin is defence in depth for the cookie-authenticated endpoints:
// browsers send Origin on POST, and it must be our own host. (SameSite=Strict
// already stops cross-site requests from carrying the cookie.)
func checkOrigin(r *http.Request) error {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return nil // not a browser, or a same-origin request from an old browser
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host != r.Host {
		return httpx.Forbidden("cross-origin request rejected")
	}
	return nil
}

func addField(fields map[string]string, name, msg string) {
	if msg != "" {
		fields[name] = msg
	}
}
