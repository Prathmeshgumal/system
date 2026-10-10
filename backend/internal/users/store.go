// Package users owns the users table: profiles, search and stored credentials.
// Other modules use it through its methods, never by querying the table.
package users

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("user not found")
	ErrEmailTaken    = errors.New("email already registered")
	ErrUsernameTaken = errors.New("username already taken")
)

// Profile is what any logged-in user may see about another user.
type Profile struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	Bio         string    `json:"bio"`
	CreatedAt   time.Time `json:"created_at"`
}

// User is the full row. Only its owner sees Email; nobody sees PasswordHash.
type User struct {
	Profile
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
}

type NewUser struct {
	Email, Username, DisplayName, PasswordHash string
}

// ProfileUpdate changes only the fields that are non-nil.
type ProfileUpdate struct {
	Username, DisplayName, Bio *string
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

const userColumns = `id, username, display_name, bio, created_at, email, password_hash`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.CreatedAt, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) Create(ctx context.Context, n NewUser) (User, error) {
	u, err := scanUser(s.db.QueryRow(ctx, `
		INSERT INTO users (email, username, display_name, password_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING `+userColumns,
		n.Email, n.Username, n.DisplayName, n.PasswordHash))
	return u, uniqueErr(err)
}

func (s *Store) GetByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
}

func (s *Store) GetByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(s.db.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email))
}

// GetMany loads many profiles in ONE query. Callers collect every id they need
// first, then call this once — the fix for the N+1 query problem.
func (s *Store) GetMany(ctx context.Context, ids []int64) (map[int64]Profile, error) {
	out := make(map[int64]Profile, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, username, display_name, bio, created_at FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	profiles, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Profile])
	if err != nil {
		return nil, err
	}
	for _, p := range profiles {
		out[p.ID] = p
	}
	return out, nil
}

// Search finds users whose username starts with prefix (already normalized).
func (s *Store) Search(ctx context.Context, prefix string, excludeID int64, limit int) ([]Profile, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, username, display_name, bio, created_at FROM users
		WHERE username LIKE $1 AND id <> $2
		ORDER BY username
		LIMIT $3`, searchPattern(prefix), excludeID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Profile])
}

func (s *Store) Update(ctx context.Context, id int64, p ProfileUpdate) (User, error) {
	// COALESCE($n, column): a NULL parameter keeps the current value.
	u, err := scanUser(s.db.QueryRow(ctx, `
		UPDATE users SET
			username     = COALESCE($2, username),
			display_name = COALESCE($3, display_name),
			bio          = COALESCE($4, bio),
			updated_at   = now()
		WHERE id = $1
		RETURNING `+userColumns,
		id, p.Username, p.DisplayName, p.Bio))
	return u, uniqueErr(err)
}

// uniqueErr turns Postgres unique violations (SQLSTATE 23505) into domain errors.
func uniqueErr(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "users_email_key":
		return ErrEmailTaken
	case "users_username_key":
		return ErrUsernameTaken
	}
	return fmt.Errorf("unexpected unique violation: %w", err)
}
