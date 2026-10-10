// Package storage keeps files (profile pictures) in S3-compatible object
// storage: AWS S3 in production, RustFS locally. See knowledge-base ADR 0004.
//
// Files never pass through the API on the way in: the browser uploads straight
// to storage with a presigned POST, whose policy the storage itself enforces.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"relay/internal/config"
)

var ErrNotFound = errors.New("object not found")

// Upload is what a browser needs to send a file: POST multipart/form-data to
// URL with every field in Fields, then the file last, in a field named "file".
type Upload struct {
	URL    string            `json:"url"`
	Fields map[string]string `json:"fields"`
}

type Info struct {
	Size        int64
	ContentType string
	ETag        string
}

// Storage is what features need. *S3 implements it; tests use a fake.
type Storage interface {
	PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (Upload, error)
	Stat(ctx context.Context, key string) (Info, error)
	Open(ctx context.Context, key string) (io.ReadCloser, Info, error)
	Delete(ctx context.Context, key string) error
}

type S3 struct {
	client    *minio.Client
	bucket    string
	publicURL *url.URL // nil: browsers use the endpoint itself
}

// New connects to S3-compatible storage. No network call happens here.
func New(cfg config.S3Config) (*S3, error) {
	// No keys → the standard AWS chain: env vars, then the EC2 instance role.
	creds := credentials.NewIAM("")
	if cfg.AccessKey != "" {
		creds = credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, "")
	}
	opts := &minio.Options{Creds: creds, Secure: cfg.UseSSL, Region: cfg.Region}
	if !strings.HasSuffix(cfg.Endpoint, "amazonaws.com") {
		// RustFS/MinIO: bucket in the path (host:9000/relay-media/key), so one
		// proxy route can forward every bucket.
		opts.BucketLookup = minio.BucketLookupPath
	}
	c, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("storage client: %w", err)
	}
	s := &S3{client: c, bucket: cfg.Bucket}
	if cfg.PublicURL != "" {
		u, err := url.Parse(cfg.PublicURL)
		if err != nil {
			return nil, fmt.Errorf("S3_PUBLIC_URL: %w", err)
		}
		s.publicURL = u
	}
	return s, nil
}

// EnsureBucket creates the bucket if it doesn't exist (local development only;
// in AWS, Terraform creates it and the API isn't allowed to).
func (s *S3) EnsureBucket(ctx context.Context) error {
	ok, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil || ok {
		return err
	}
	return s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
}

// PresignUpload returns a POST policy that allows exactly one upload: this key,
// this content type, at most maxBytes, before ttl runs out. Storage checks all
// of it — a presigned PUT couldn't limit the size.
func (s *S3) PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (Upload, error) {
	p := minio.NewPostPolicy()
	for _, err := range []error{
		p.SetBucket(s.bucket),
		p.SetKey(key),
		p.SetContentType(contentType),
		p.SetContentLengthRange(1, maxBytes),
		p.SetExpires(time.Now().UTC().Add(ttl)),
	} {
		if err != nil {
			return Upload{}, err
		}
	}
	u, fields, err := s.client.PresignedPostPolicy(ctx, p)
	if err != nil {
		return Upload{}, err
	}
	if s.publicURL != nil {
		// Same path, different base: e.g. http://rustfs:9000/relay-media → /storage/relay-media.
		u.Path = strings.TrimSuffix(s.publicURL.Path, "/") + u.Path
		u.Scheme, u.Host = s.publicURL.Scheme, s.publicURL.Host
	}
	return Upload{URL: u.String(), Fields: fields}, nil
}

func (s *S3) Stat(ctx context.Context, key string) (Info, error) {
	st, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return Info{}, notFound(err)
	}
	return Info{Size: st.Size, ContentType: st.ContentType, ETag: st.ETag}, nil
}

func (s *S3) Open(ctx context.Context, key string) (io.ReadCloser, Info, error) {
	info, err := s.Stat(ctx, key)
	if err != nil {
		return nil, Info{}, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, Info{}, notFound(err)
	}
	return obj, info, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
}

func notFound(err error) error {
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return ErrNotFound
	}
	return err
}
