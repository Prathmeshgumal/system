package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"relay/internal/auth"
	"relay/internal/testdb"
	"relay/internal/users"
)

func TestSessions_RotateAndReuseDetection(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	u, err := users.NewStore(db).Create(ctx, users.NewUser{Email: "a@x.io", Username: "alice", DisplayName: "A", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	s := auth.NewSessions(db, time.Hour)

	sess, tok1, err := s.Create(ctx, u.ID, "test")
	if err != nil {
		t.Fatal(err)
	}

	sess2, tok2, err := s.Rotate(ctx, tok1)
	if err != nil {
		t.Fatalf("first rotate: %v", err)
	}
	if sess2.ID != sess.ID || sess2.UserID != u.ID || tok2 == tok1 {
		t.Fatalf("rotate gave %+v / same token=%v", sess2, tok2 == tok1)
	}

	// Replaying tok1 (already rotated away) must revoke the session…
	if _, _, err := s.Rotate(ctx, tok1); !errors.Is(err, auth.ErrTokenReused) {
		t.Fatalf("replay of old token: err = %v, want ErrTokenReused", err)
	}
	// …so even the legitimate newest token stops working.
	if _, _, err := s.Rotate(ctx, tok2); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("token after revocation: err = %v, want ErrInvalidToken", err)
	}
}

func TestSessions_RevokeAndUnknown(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	u, _ := users.NewStore(db).Create(ctx, users.NewUser{Email: "a@x.io", Username: "alice", DisplayName: "A", PasswordHash: "x"})
	s := auth.NewSessions(db, time.Hour)

	_, tok, _ := s.Create(ctx, u.ID, "")
	if err := s.Revoke(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Rotate(ctx, tok); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("rotate after logout: err = %v", err)
	}
	if _, _, err := s.Rotate(ctx, "made-up"); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("unknown token: err = %v", err)
	}
}

func TestSessions_Expired(t *testing.T) {
	db := testdb.New(t)
	ctx := context.Background()
	u, _ := users.NewStore(db).Create(ctx, users.NewUser{Email: "a@x.io", Username: "alice", DisplayName: "A", PasswordHash: "x"})
	s := auth.NewSessions(db, time.Hour)
	_, tok, _ := s.Create(ctx, u.ID, "")

	if _, err := db.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Rotate(ctx, tok); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("expired session: err = %v", err)
	}
}
