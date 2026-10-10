package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidToken = errors.New("refresh token is invalid or expired")
	ErrTokenReused  = errors.New("refresh token was already used — session revoked")
)

// Session is one login on one device.
type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
}

// Sessions stores refresh-token sessions. Every refresh ROTATES the token:
// the old one stops working and a new one is issued. If an old token ever
// comes back, someone kept a copy (a thief, or a bug) — so we revoke the
// whole session. See ADR 0001.
type Sessions struct {
	db  *pgxpool.Pool
	ttl time.Duration
}

func NewSessions(db *pgxpool.Pool, ttl time.Duration) *Sessions {
	return &Sessions{db: db, ttl: ttl}
}

// newRefreshToken returns 256 random bits for the cookie and the SHA-256 we store.
// A fast hash is fine here (unlike passwords): nobody can guess 256 random bits.
func newRefreshToken() (string, []byte) {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand never fails on supported platforms
	tok := base64.RawURLEncoding.EncodeToString(b)
	return tok, hashToken(tok)
}

func hashToken(tok string) []byte {
	h := sha256.Sum256([]byte(tok))
	return h[:]
}

// Create starts a session and returns its refresh token.
func (s *Sessions) Create(ctx context.Context, userID int64, userAgent string) (Session, string, error) {
	tok, hash := newRefreshToken()
	sess := Session{UserID: userID}
	err := s.db.QueryRow(ctx, `
		INSERT INTO sessions (user_id, token_hash, user_agent, expires_at)
		VALUES ($1, $2, $3, now() + $4::interval)
		RETURNING id, expires_at`,
		userID, hash, truncate(userAgent, 256), s.ttl).Scan(&sess.ID, &sess.ExpiresAt)
	return sess, tok, err
}

// Rotate swaps a valid refresh token for a new one and extends the session.
func (s *Sessions) Rotate(ctx context.Context, token string) (Session, string, error) {
	old := hashToken(token)
	newTok, newHash := newRefreshToken()

	var sess Session
	err := s.db.QueryRow(ctx, `
		UPDATE sessions
		SET prev_token_hash = token_hash, token_hash = $2,
		    last_used_at = now(), expires_at = now() + $3::interval
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING id, user_id, expires_at`,
		old, newHash, s.ttl).Scan(&sess.ID, &sess.UserID, &sess.ExpiresAt)
	if err == nil {
		return sess, newTok, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, "", err
	}

	// Not a current token. Is it the one we replaced? Then it's being replayed.
	var id string
	err = s.db.QueryRow(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE prev_token_hash = $1 AND revoked_at IS NULL
		RETURNING id`, old).Scan(&id)
	if err == nil {
		slog.Warn("refresh token reuse detected, session revoked", "session_id", id)
		return Session{}, "", ErrTokenReused
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Session{}, "", err
	}
	return Session{}, "", ErrInvalidToken
}

// Revoke ends the session that owns token (logout). Unknown tokens are ignored.
func (s *Sessions) Revoke(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL`, hashToken(token))
	return err
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
