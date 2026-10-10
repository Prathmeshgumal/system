package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"relay/internal/config"
	"relay/internal/storage"
)

// newS3 needs a running S3-compatible server, e.g.
//
//	docker compose up -d storage
//	TEST_S3_ENDPOINT=localhost:9000 go test ./internal/storage/
func newS3(t *testing.T) *storage.S3 {
	t.Helper()
	ep := os.Getenv("TEST_S3_ENDPOINT")
	if ep == "" {
		t.Skip("TEST_S3_ENDPOINT not set — skipping storage integration test")
	}
	s, err := storage.New(config.S3Config{
		Endpoint: ep, Region: "us-east-1", Bucket: "relay-test",
		AccessKey: envOr("TEST_S3_ACCESS_KEY", "relay"), SecretKey: envOr("TEST_S3_SECRET_KEY", "relay-dev-secret"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureBucket(context.Background()); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	return s
}

func envOr(k, v string) string {
	if e := os.Getenv(k); e != "" {
		return e
	}
	return v
}

// post does what the browser does with an Upload.
func post(t *testing.T, up storage.Upload, contentType string, body []byte) int {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range up.Fields {
		_ = w.WriteField(k, v)
	}
	_ = w.WriteField("Content-Type", contentType)
	fw, _ := w.CreateFormFile("file", "pic") // the file must be the LAST field
	_, _ = fw.Write(body)
	_ = w.Close()
	res, err := http.Post(up.URL, w.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestPresignedPost_EnforcesSizeAndType(t *testing.T) {
	s := newS3(t)
	ctx := context.Background()
	key := "avatars/test/" + time.Now().Format("150405.000000") + ".jpg"

	up, err := s.PresignUpload(ctx, key, "image/jpeg", 1000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if code := post(t, up, "image/jpeg", make([]byte, 5000)); code < 400 {
		t.Errorf("5000 bytes with a 1000-byte limit = %d, want rejected", code)
	}
	if code := post(t, up, "text/html", make([]byte, 100)); code < 400 {
		t.Errorf("wrong content type = %d, want rejected", code)
	}
	if code := post(t, up, "image/jpeg", []byte(strings.Repeat("x", 500))); code >= 300 {
		t.Fatalf("valid upload = %d", code)
	}

	info, err := s.Stat(ctx, key)
	if err != nil || info.Size != 500 || info.ContentType != "image/jpeg" {
		t.Fatalf("stat = %+v, %v", info, err)
	}
	rc, _, err := s.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if len(b) != 500 {
		t.Errorf("read %d bytes", len(b))
	}

	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Errorf("stat after delete: %v, want ErrNotFound", err)
	}
}

func TestPresignUpload_PublicURLRewritesTheBase(t *testing.T) {
	s, err := storage.New(config.S3Config{
		Endpoint: "rustfs:9000", Region: "us-east-1", Bucket: "relay-media",
		AccessKey: "k", SecretKey: "s", PublicURL: "/storage",
	})
	if err != nil {
		t.Fatal(err)
	}
	up, err := s.PresignUpload(context.Background(), "avatars/1/a.jpg", "image/jpeg", 10, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if up.URL != "/storage/relay-media/" {
		t.Errorf("URL = %q, want /storage/relay-media/ (same origin, through the proxy)", up.URL)
	}
	if up.Fields["key"] != "avatars/1/a.jpg" || up.Fields["policy"] == "" || up.Fields["x-amz-signature"] == "" {
		t.Errorf("fields = %v", up.Fields)
	}
}
