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
	AvatarURL   *string   `json:"avatar_url"` // nil = no picture (the app shows initials)
	AvatarKey   *string   `json:"-"`          // where the picture is in object storage
}

// MediaPrefix is where the API serves stored files (see users.MediaHandler).
const MediaPrefix = "/media/"

func (p *Profile) setAvatar(key *string) {
	p.AvatarKey = key
	p.AvatarURL = nil
	if key != nil {
		u := MediaPrefix + *key
		p.AvatarURL = &u
	}
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

const (
	profileColumns = `id, username, display_name, bio, created_at, avatar_key`
	userColumns    = profileColumns + `, email, password_hash`
)

func scanProfile(row pgx.Row) (Profile, error) {
	var p Profile
	var key *string
	if err := row.Scan(&p.ID, &p.Username, &p.DisplayName, &p.Bio, &p.CreatedAt, &key); err != nil {
		return Profile{}, err
	}
	p.setAvatar(key)
	return p, nil
}

func scanUser(row pgx.Row) (User, error) {
	var u User
	var key *string
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.CreatedAt, &key, &u.Email, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	u.setAvatar(key)
	return u, err
}

func collectProfiles(rows pgx.Rows) ([]Profile, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Profile, error) { return scanProfile(r) })
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
		SELECT `+profileColumns+` FROM users WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	profiles, err := collectProfiles(rows)
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
		SELECT `+profileColumns+` FROM users
		WHERE username LIKE $1 AND id <> $2
		ORDER BY username
		LIMIT $3`, searchPattern(prefix), excludeID, limit)
	if err != nil {
		return nil, err
	}
	return collectProfiles(rows)
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

// SetAvatar points the user at a new picture (or none, with nil) and returns
// the previous key, so the caller can delete the old file. One statement: the
// CTE locks the row, so two uploads at once can't both see the same "old" key.
//
// The CTE must be joined in FROM. Referenced only in RETURNING, Postgres runs it
// after the UPDATE — and a locking read skips rows the same statement changed.
func (s *Store) SetAvatar(ctx context.Context, id int64, key *string) (User, *string, error) {
	var u User
	var newKey, old *string
	err := s.db.QueryRow(ctx, `
		WITH old AS (SELECT id, avatar_key FROM users WHERE id = $1 FOR UPDATE)
		UPDATE users u SET avatar_key = $2, updated_at = now()
		FROM old WHERE u.id = old.id
		RETURNING u.id, u.username, u.display_name, u.bio, u.created_at, u.avatar_key,
		          u.email, u.password_hash, old.avatar_key`, id, key).
		Scan(&u.ID, &u.Username, &u.DisplayName, &u.Bio, &u.CreatedAt, &newKey, &u.Email, &u.PasswordHash, &old)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, nil, ErrNotFound
	}
	if err != nil {
		return User{}, nil, err
	}
	u.setAvatar(newKey)
	return u, old, nil
}
