// Package auth owns logging in: password hashing, access tokens, WebSocket
// tickets and refresh-token sessions. Design: knowledge-base/adr/0001-auth-tokens.md.
package auth

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ErrBusy means too many logins are queued for hashing; the client should retry shortly.
var ErrBusy = errors.New("too many logins at once, try again")

// ValidatePassword returns a message if the password can't be used.
// The 72-byte cap is bcrypt's: it silently ignores everything after byte 72,
// so "<72 bytes>A" and "<72 bytes>B" would be the same password.
func ValidatePassword(p string) string {
	switch {
	case len(p) < 8:
		return "must be at least 8 characters"
	case len(p) > 72:
		return "must be at most 72 bytes"
	}
	return ""
}

// Hasher hashes and checks passwords with bcrypt.
//
// bcrypt is deliberately slow (~250 ms of one core at cost 12). Part 1.4 found
// that 50 logins at once took every core for ~3 s and stalled message sending
// for everyone. So hashing runs in a fixed number of slots (a "bulkhead"):
// logins queue for a slot, and the remaining cores stay free for chat.
type Hasher struct {
	cost     int
	dummy    []byte        // a real hash of nothing in particular, see VerifyNobody
	slots    chan struct{} // one token per hash allowed to run at once
	maxWait  time.Duration // queueing longer than this → ErrBusy (load shedding)
	generate func(password []byte, cost int) ([]byte, error)
}

// NewHasher allows `concurrency` hashes at once (main uses GOMAXPROCS − 1).
func NewHasher(cost, concurrency int) *Hasher {
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy password for timing"), cost)
	if err != nil {
		panic(err) // only fails for an invalid cost, which config already rejects
	}
	return &Hasher{
		cost:     cost,
		dummy:    dummy,
		slots:    make(chan struct{}, max(1, concurrency)),
		maxWait:  10 * time.Second,
		generate: bcrypt.GenerateFromPassword,
	}
}

// Hash returns a bcrypt hash: "$2a$<cost>$<22-char salt><31-char hash>".
// The salt and cost are stored inside the hash, so Verify needs nothing else.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	var b []byte
	err := h.inSlot(ctx, func() (err error) {
		b, err = h.generate([]byte(password), h.cost)
		return err
	})
	return string(b), err
}

// Verify reports whether password matches hash. The error is only for
// "couldn't check right now" (ErrBusy, or the client went away).
func (h *Hasher) Verify(ctx context.Context, hash, password string) (bool, error) {
	var ok bool
	err := h.inSlot(ctx, func() error {
		ok = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
		return nil
	})
	return ok, err
}

// VerifyNobody burns the same CPU time as Verify for a login with an unknown
// email. Without it, "no such user" answers in 1 ms and "wrong password" in
// 250 ms — and an attacker can tell which emails are registered.
func (h *Hasher) VerifyNobody(ctx context.Context, password string) error {
	return h.inSlot(ctx, func() error {
		_ = bcrypt.CompareHashAndPassword(h.dummy, []byte(password))
		return nil
	})
}

// inSlot waits for a free slot (at most maxWait), runs fn, and frees the slot.
func (h *Hasher) inSlot(ctx context.Context, fn func() error) error {
	wait := time.NewTimer(h.maxWait)
	defer wait.Stop()
	select {
	case h.slots <- struct{}{}:
	case <-wait.C:
		return ErrBusy
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-h.slots }()
	return fn()
}
