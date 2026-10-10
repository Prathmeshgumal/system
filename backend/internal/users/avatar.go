package users

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"relay/internal/httpx"
	"relay/internal/storage"
)

const (
	maxAvatarBytes = 1 << 20 // 1 MiB; the browser shrinks pictures to 256×256 first (~20 KB)
	uploadTTL      = 5 * time.Minute
)

// Only raster images. An SVG is a document that can run scripts, and we serve
// pictures from our own origin — so it's never allowed.
var avatarTypes = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

// AvatarHandler: profile pictures. Uploading is three steps (ADR 0004):
//
//  1. POST /api/me/avatar/upload → a presigned POST policy for one new key
//  2. the browser POSTs the file straight to storage
//  3. PUT /api/me/avatar {key}   → we check the file, then point the user at it
//
// files may be nil (no S3_ENDPOINT configured): the endpoints answer 503.
type AvatarHandler struct {
	store *Store
	files storage.Storage
}

func NewAvatarHandler(s *Store, files storage.Storage) *AvatarHandler {
	return &AvatarHandler{store: s, files: files}
}

// RegisterPrivate adds the endpoints that need a logged-in user.
func (h *AvatarHandler) RegisterPrivate(mux *http.ServeMux) {
	mux.Handle("POST /api/me/avatar/upload", httpx.Handle(h.startUpload))
	mux.Handle("PUT /api/me/avatar", httpx.Handle(h.confirm))
	mux.Handle("DELETE /api/me/avatar", httpx.Handle(h.remove))
}

// RegisterPublic adds GET /media/…: <img> tags can't send an access token, so
// pictures are public — the keys are long and random, nobody can guess them.
func (h *AvatarHandler) RegisterPublic(mux *http.ServeMux) {
	mux.Handle("GET "+MediaPrefix+"{key...}", httpx.Handle(h.serve))
}

func (h *AvatarHandler) startUpload(w http.ResponseWriter, r *http.Request) error {
	if h.files == nil {
		return errNoStorage
	}
	var in struct {
		ContentType string `json:"content_type"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	ext, ok := avatarTypes[in.ContentType]
	if !ok {
		return httpx.Invalid(map[string]string{"content_type": "must be image/jpeg, image/png or image/webp"})
	}
	key := fmt.Sprintf("avatars/%d/%s%s", httpx.UserID(r.Context()), randomHex(16), ext)
	up, err := h.files.PresignUpload(r.Context(), key, in.ContentType, maxAvatarBytes, uploadTTL)
	if err != nil {
		return err
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"key": key, "upload": up})
	return nil
}

func (h *AvatarHandler) confirm(w http.ResponseWriter, r *http.Request) error {
	if h.files == nil {
		return errNoStorage
	}
	var in struct {
		Key string `json:"key"`
	}
	if err := httpx.ReadJSON(w, r, &in); err != nil {
		return err
	}
	me := httpx.UserID(r.Context())
	// Only keys we handed to *this* user: nobody can claim someone else's file.
	if !strings.HasPrefix(in.Key, fmt.Sprintf("avatars/%d/", me)) || strings.Contains(in.Key, "..") || len(in.Key) > 200 {
		return httpx.Invalid(map[string]string{"key": "not an upload of yours"})
	}
	// Don't trust "I uploaded it": look at what's actually there.
	info, err := h.files.Stat(r.Context(), in.Key)
	if errors.Is(err, storage.ErrNotFound) {
		return httpx.BadRequest("upload_missing", "the file hasn't arrived — upload it first")
	}
	if err != nil {
		return err
	}
	if _, ok := avatarTypes[info.ContentType]; !ok || info.Size > maxAvatarBytes {
		_ = h.files.Delete(r.Context(), in.Key)
		return httpx.BadRequest("bad_upload", "the file must be a JPEG, PNG or WebP of at most 1 MB")
	}

	key := in.Key
	u, old, err := h.store.SetAvatar(r.Context(), me, &key)
	if err != nil {
		return err
	}
	h.deleteOld(r, old, key)
	httpx.WriteJSON(w, http.StatusOK, u)
	return nil
}

func (h *AvatarHandler) remove(w http.ResponseWriter, r *http.Request) error {
	u, old, err := h.store.SetAvatar(r.Context(), httpx.UserID(r.Context()), nil)
	if err != nil {
		return err
	}
	h.deleteOld(r, old, "")
	httpx.WriteJSON(w, http.StatusOK, u)
	return nil
}

// deleteOld removes the previous picture. Failing here only leaves an orphan
// file behind, so it's logged, not returned.
func (h *AvatarHandler) deleteOld(r *http.Request, old *string, current string) {
	if old == nil || *old == current || h.files == nil {
		return
	}
	if err := h.files.Delete(r.Context(), *old); err != nil {
		slog.Warn("delete old avatar", "key", *old, "err", err)
	}
}

// serve streams a stored picture. Keys never change (a new picture gets a new
// key), so browsers and CDNs may cache them forever.
func (h *AvatarHandler) serve(w http.ResponseWriter, r *http.Request) error {
	key := r.PathValue("key")
	if h.files == nil || !strings.HasPrefix(key, "avatars/") || strings.Contains(key, "..") {
		return httpx.NotFound("not found")
	}
	body, info, err := h.files.Open(r.Context(), key)
	if errors.Is(err, storage.ErrNotFound) {
		return httpx.NotFound("not found")
	}
	if err != nil {
		return err
	}
	defer body.Close()

	etag := `"` + strings.Trim(info.ETag, `"`) + `"`
	hdr := w.Header()
	hdr.Set("Cache-Control", "public, max-age=31536000, immutable")
	hdr.Set("ETag", etag)
	hdr.Set("X-Content-Type-Options", "nosniff") // never let a browser guess "this is HTML"
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return nil
	}
	ct := info.ContentType
	if _, ok := avatarTypes[ct]; !ok {
		ct = "application/octet-stream"
	}
	hdr.Set("Content-Type", ct)
	hdr.Set("Content-Length", fmt.Sprint(info.Size))
	_, _ = io.Copy(w, body) // a client hanging up mid-download isn't our error
	return nil
}

var errNoStorage = httpx.NewError(http.StatusServiceUnavailable, "storage_off",
	"profile pictures are switched off on this server (no S3_ENDPOINT)")

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
