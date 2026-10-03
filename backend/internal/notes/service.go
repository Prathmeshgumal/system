package notes

import (
	"context"
	"strings"
	"unicode/utf8"
)

const (
	maxBodyChars = 1000 // same limit as the CHECK constraint in the migration
	listLimit    = 20
)

// ValidationError lists every invalid field with a human-readable reason.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string { return "validation failed" }

// store is what the service needs from the database layer. *Store satisfies
// it; tests use a fake.
type store interface {
	Create(ctx context.Context, body string) (Note, error)
	ListLatest(ctx context.Context, limit int32) ([]Note, error)
}

// Service holds the rules. It knows nothing about HTTP or SQL.
type Service struct {
	store store
}

func NewService(s store) *Service {
	return &Service{store: s}
}

func (s *Service) Create(ctx context.Context, body string) (Note, error) {
	if msg := validateBody(body); msg != "" {
		return Note{}, &ValidationError{Fields: map[string]string{"body": msg}}
	}
	return s.store.Create(ctx, body)
}

func (s *Service) ListLatest(ctx context.Context) ([]Note, error) {
	return s.store.ListLatest(ctx, listLimit)
}

// validateBody counts characters (runes), not bytes — the same way Postgres's
// char_length does — so "é" counts as one.
func validateBody(body string) string {
	switch {
	case strings.TrimSpace(body) == "":
		return "must not be empty"
	case utf8.RuneCountInString(body) > maxBodyChars:
		return "must be at most 1000 characters"
	}
	return ""
}
