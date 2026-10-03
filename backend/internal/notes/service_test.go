package notes

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeStore records what the service asked for, without a database.
type fakeStore struct {
	created   []string
	listLimit int32
	err       error
}

func (f *fakeStore) Create(ctx context.Context, body string) (Note, error) {
	if f.err != nil {
		return Note{}, f.err
	}
	f.created = append(f.created, body)
	return Note{ID: 1, Body: body, CreatedAt: time.Unix(0, 0)}, nil
}

func (f *fakeStore) ListLatest(ctx context.Context, limit int32) ([]Note, error) {
	f.listLimit = limit
	return []Note{}, f.err
}

func TestServiceCreate_BodyLength(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		valid bool
	}{
		{"empty", "", false},
		{"whitespace only", "   \n\t", false},
		{"one char", "a", true},
		{"exactly 1000", strings.Repeat("a", 1000), true},
		{"1001", strings.Repeat("a", 1001), false},
		// Postgres counts characters, not bytes: 1000 × "é" (2 bytes each) is still valid.
		{"1000 multi-byte chars", strings.Repeat("é", 1000), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeStore{}
			svc := NewService(store)

			_, err := svc.Create(context.Background(), tt.body)

			var verr *ValidationError
			if tt.valid {
				if err != nil {
					t.Fatalf("Create() error = %v, want nil", err)
				}
				if len(store.created) != 1 {
					t.Fatal("valid note was not saved")
				}
				return
			}
			if !errors.As(err, &verr) || verr.Fields["body"] == "" {
				t.Fatalf("Create() error = %v, want *ValidationError with a body message", err)
			}
			if len(store.created) != 0 {
				t.Fatal("invalid note reached the store")
			}
		})
	}
}

func TestServiceListLatest_AsksForTwenty(t *testing.T) {
	store := &fakeStore{}
	if _, err := NewService(store).ListLatest(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.listLimit != 20 {
		t.Fatalf("limit = %d, want 20", store.listLimit)
	}
}

func TestServiceCreate_StoreError_IsReturned(t *testing.T) {
	store := &fakeStore{err: ErrUnavailable}
	_, err := NewService(store).Create(context.Background(), "hi")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}
