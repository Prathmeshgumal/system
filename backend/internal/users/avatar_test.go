package users_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"relay/internal/httpx"
	"relay/internal/storage"
	"relay/internal/testdb"
	"relay/internal/users"
)

// fakeStorage keeps objects in memory. "Uploading" = calling put directly.
type fakeStorage struct {
	mu      sync.Mutex
	objects map[string]storage.Info
	deleted []string
}

func (f *fakeStorage) PresignUpload(_ context.Context, key, ct string, max int64, _ time.Duration) (storage.Upload, error) {
	return storage.Upload{URL: "/storage/bucket/", Fields: map[string]string{"key": key, "Content-Type": ct}}, nil
}
func (f *fakeStorage) put(key, ct string, size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = storage.Info{Size: size, ContentType: ct, ETag: "abc"}
}
func (f *fakeStorage) Stat(_ context.Context, key string) (storage.Info, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.objects[key]
	if !ok {
		return storage.Info{}, storage.ErrNotFound
	}
	return i, nil
}
func (f *fakeStorage) Open(ctx context.Context, key string) (io.ReadCloser, storage.Info, error) {
	i, err := f.Stat(ctx, key)
	if err != nil {
		return nil, storage.Info{}, err
	}
	return io.NopCloser(strings.NewReader(strings.Repeat("x", int(i.Size)))), i, nil
}
func (f *fakeStorage) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	f.deleted = append(f.deleted, key)
	return nil
}

type avatarEnv struct {
	t     *testing.T
	mux   *http.ServeMux
	files *fakeStorage
	me    int64
}

func newAvatarEnv(t *testing.T) *avatarEnv {
	s := users.NewStore(testdb.New(t))
	files := &fakeStorage{objects: map[string]storage.Info{}}
	h := users.NewAvatarHandler(s, files)
	mux := http.NewServeMux()
	h.RegisterPrivate(mux)
	h.RegisterPublic(mux)
	return &avatarEnv{t: t, mux: mux, files: files, me: newUser(t, s, "alice").ID}
}

func (e *avatarEnv) do(method, path string, body any, out any) *httptest.ResponseRecorder {
	e.t.Helper()
	var r io.Reader = http.NoBody
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	req = req.WithContext(httpx.WithUserID(req.Context(), e.me))
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	if out != nil {
		_ = json.Unmarshal(rec.Body.Bytes(), out)
	}
	return rec
}

func TestAvatar_UploadConfirmServeReplace(t *testing.T) {
	e := newAvatarEnv(t)

	var start struct {
		Key    string
		Upload storage.Upload
	}
	if rec := e.do("POST", "/api/me/avatar/upload", map[string]string{"content_type": "image/jpeg"}, &start); rec.Code != 200 {
		t.Fatalf("start = %d %s", rec.Code, rec.Body)
	}
	if !strings.HasPrefix(start.Key, "avatars/") || !strings.HasSuffix(start.Key, ".jpg") || start.Upload.URL == "" {
		t.Fatalf("start = %+v", start)
	}

	// Confirming before the file arrived fails.
	if rec := e.do("PUT", "/api/me/avatar", map[string]string{"key": start.Key}, nil); rec.Code != 400 {
		t.Errorf("confirm before upload = %d, want 400", rec.Code)
	}

	e.files.put(start.Key, "image/jpeg", 20_000) // the browser's POST
	var me struct {
		AvatarURL *string `json:"avatar_url"`
	}
	if rec := e.do("PUT", "/api/me/avatar", map[string]string{"key": start.Key}, &me); rec.Code != 200 {
		t.Fatalf("confirm = %d %s", rec.Code, rec.Body)
	}
	if me.AvatarURL == nil || *me.AvatarURL != "/media/"+start.Key {
		t.Fatalf("avatar_url = %v", me.AvatarURL)
	}

	// Served with long-lived caching, and revalidates with the ETag.
	rec := e.do("GET", *me.AvatarURL, nil, nil)
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/jpeg" || rec.Body.Len() != 20_000 ||
		!strings.Contains(rec.Header().Get("Cache-Control"), "immutable") || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("serve = %d %v", rec.Code, rec.Header())
	}
	req := httptest.NewRequest("GET", *me.AvatarURL, nil)
	req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	rec2 := httptest.NewRecorder()
	e.mux.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusNotModified {
		t.Errorf("revalidate = %d, want 304", rec2.Code)
	}

	// A second picture replaces the first, and the old file is deleted.
	var second struct{ Key string }
	e.do("POST", "/api/me/avatar/upload", map[string]string{"content_type": "image/png"}, &second)
	e.files.put(second.Key, "image/png", 100)
	e.do("PUT", "/api/me/avatar", map[string]string{"key": second.Key}, nil)
	if len(e.files.deleted) != 1 || e.files.deleted[0] != start.Key {
		t.Errorf("deleted = %v, want the first picture", e.files.deleted)
	}

	if rec := e.do("DELETE", "/api/me/avatar", nil, &me); rec.Code != 200 || me.AvatarURL != nil {
		t.Errorf("remove = %d %v", rec.Code, me.AvatarURL)
	}
}

func TestAvatar_Rejections(t *testing.T) {
	e := newAvatarEnv(t)

	if rec := e.do("POST", "/api/me/avatar/upload", map[string]string{"content_type": "image/svg+xml"}, nil); rec.Code != 400 {
		t.Errorf("svg = %d, want 400 (SVG can carry scripts)", rec.Code)
	}
	// Someone else's file, or a path trick.
	for _, key := range []string{"avatars/999/x.jpg", "avatars/1/../999/x.jpg", "other/x.jpg"} {
		e.files.put(key, "image/jpeg", 10)
		if rec := e.do("PUT", "/api/me/avatar", map[string]string{"key": key}, nil); rec.Code != 400 {
			t.Errorf("claim %q = %d, want 400", key, rec.Code)
		}
	}
	// What's in storage is checked, not what the browser said.
	var start struct{ Key string }
	e.do("POST", "/api/me/avatar/upload", map[string]string{"content_type": "image/jpeg"}, &start)
	e.files.put(start.Key, "text/html", 10)
	if rec := e.do("PUT", "/api/me/avatar", map[string]string{"key": start.Key}, nil); rec.Code != 400 {
		t.Errorf("html posing as an upload = %d, want 400", rec.Code)
	}
	if _, err := e.files.Stat(context.Background(), start.Key); err == nil {
		t.Error("a rejected upload should be deleted")
	}
	if rec := e.do("GET", "/media/other/x.jpg", nil, nil); rec.Code != 404 {
		t.Errorf("serving outside avatars/ = %d, want 404", rec.Code)
	}
}

func TestAvatar_NoStorageConfigured(t *testing.T) {
	s := users.NewStore(testdb.New(t))
	mux := http.NewServeMux()
	users.NewAvatarHandler(s, nil).RegisterPrivate(mux)
	req := httptest.NewRequest("POST", "/api/me/avatar/upload", strings.NewReader(`{"content_type":"image/jpeg"}`))
	req = req.WithContext(httpx.WithUserID(req.Context(), 1))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("no storage = %d, want 503", rec.Code)
	}
}
