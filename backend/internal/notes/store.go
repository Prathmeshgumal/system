// Package notes is a throwaway demo feature (Part 0.2, decision L) that
// exercises the full handler → service → store pattern every real feature
// will use. It is deleted at the start of Part 1.1.
package notes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"relay/internal/notes/db"
	"relay/internal/platform/database"
)

// Note is what the API returns. It's our own type rather than the generated
// db.Note, so the JSON the frontend sees doesn't change if the table does.
type Note struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrUnavailable means the database couldn't be reached. The handler turns it
// into 503, as opposed to 500 for genuine bugs.
var ErrUnavailable = errors.New("database unavailable")

// Store reads and writes notes using the sqlc-generated queries.
type Store struct {
	q *db.Queries
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{q: db.New(pool)}
}

func (s *Store) Create(ctx context.Context, body string) (Note, error) {
	row, err := s.q.CreateNote(ctx, body)
	if err != nil {
		return Note{}, wrap("create note", err)
	}
	return toNote(row), nil
}

func (s *Store) ListLatest(ctx context.Context, limit int32) ([]Note, error) {
	rows, err := s.q.ListLatestNotes(ctx, limit)
	if err != nil {
		return nil, wrap("list notes", err)
	}
	out := make([]Note, 0, len(rows)) // never nil: encodes as [] not null
	for _, r := range rows {
		out = append(out, toNote(r))
	}
	return out, nil
}

func toNote(r db.Note) Note {
	return Note{ID: r.ID, Body: r.Body, CreatedAt: r.CreatedAt}
}

// wrap adds context to an error, and marks "couldn't reach the database"
// errors with ErrUnavailable so callers can tell them apart from bugs.
func wrap(op string, err error) error {
	if database.IsUnavailable(err) {
		return fmt.Errorf("%s: %w: %w", op, ErrUnavailable, err)
	}
	return fmt.Errorf("%s: %w", op, err)
}
