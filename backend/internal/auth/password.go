// Package auth owns logging in: password hashing, access tokens, WebSocket
// tickets and refresh-token sessions. Design: knowledge-base/adr/0001-auth-tokens.md.
package auth

import (
	"golang.org/x/crypto/bcrypt"
)

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
type Hasher struct {
	cost  int
	dummy []byte // a real hash of nothing in particular, see VerifyNobody
}

func NewHasher(cost int) *Hasher {
	dummy, err := bcrypt.GenerateFromPassword([]byte("dummy password for timing"), cost)
	if err != nil {
		panic(err) // only fails for an invalid cost, which config already rejects
	}
	return &Hasher{cost: cost, dummy: dummy}
}

// Hash returns a bcrypt hash: "$2a$<cost>$<22-char salt><31-char hash>".
// The salt and cost are stored inside the hash, so Verify needs nothing else.
func (h *Hasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	return string(b), err
}

func (h *Hasher) Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// VerifyNobody burns the same CPU time as Verify for a login with an unknown
// email. Without it, "no such user" answers in 1 ms and "wrong password" in
// 250 ms — and an attacker can tell which emails are registered.
func (h *Hasher) VerifyNobody(password string) {
	_ = bcrypt.CompareHashAndPassword(h.dummy, []byte(password))
}
