package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var secret = []byte(strings.Repeat("s", 32))

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestTokens_AccessRoundTrip(t *testing.T) {
	tk := NewTokens(secret, 15*time.Minute)
	tok, exp, err := tk.IssueAccess(42, "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(exp); d < 14*time.Minute || d > 15*time.Minute {
		t.Errorf("expires in %v, want ~15m", d)
	}
	c, err := tk.ParseAccess(tok)
	if err != nil || c.UserID != 42 || c.SessionID != "sess-1" {
		t.Fatalf("ParseAccess = %+v, %v", c, err)
	}
}

func TestTokens_AccessRejected(t *testing.T) {
	tk := NewTokens(secret, 15*time.Minute)
	good, _, _ := tk.IssueAccess(42, "s")
	ticket, _ := tk.IssueTicket(42)

	expired := NewTokens(secret, 15*time.Minute)
	expired.now = fixedClock(time.Now().Add(-time.Hour))
	old, _, _ := expired.IssueAccess(42, "s")

	other, _, _ := NewTokens([]byte(strings.Repeat("o", 32)), time.Minute).IssueAccess(42, "s")

	// A token that says "no signature needed" — the classic JWT attack.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "42", "aud": "api", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)

	cases := map[string]string{
		"expired":             old,
		"wrong secret":        other,
		"alg none":            none,
		"ws ticket as access": ticket,
		"tampered":            good[:len(good)-2] + "xx",
		"garbage":             "not.a.jwt",
	}
	for name, tok := range cases {
		if _, err := tk.ParseAccess(tok); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestTokens_Ticket(t *testing.T) {
	tk := NewTokens(secret, 15*time.Minute)
	ticket, err := tk.IssueTicket(7)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := tk.ParseTicket(ticket); err != nil || id != 7 {
		t.Fatalf("ParseTicket = %d, %v", id, err)
	}
	access, _, _ := tk.IssueAccess(7, "s")
	if _, err := tk.ParseTicket(access); err == nil {
		t.Error("access token accepted as a WebSocket ticket")
	}

	later := NewTokens(secret, 15*time.Minute)
	later.now = fixedClock(time.Now().Add(2 * time.Minute))
	if _, err := later.ParseTicket(ticket); err == nil {
		t.Error("ticket still valid after 2 minutes")
	}
}
