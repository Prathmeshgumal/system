package auth

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"short":                 false,
		"12345678":              true,
		strings.Repeat("a", 72): true,
		strings.Repeat("a", 73): false, // bcrypt ignores bytes after 72
		strings.Repeat("é", 37): false, // 37 characters but 74 bytes
	}
	for pw, ok := range cases {
		if msg := ValidatePassword(pw); (msg == "") != ok {
			t.Errorf("ValidatePassword(len %d bytes) = %q, want ok=%v", len(pw), msg, ok)
		}
	}
}

func TestHasher_HashAndVerify(t *testing.T) {
	h := NewHasher(bcrypt.MinCost)
	hash, err := h.Hash("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse" || !strings.HasPrefix(hash, "$2a$04$") {
		t.Errorf("hash = %q, want a bcrypt hash with cost 04", hash)
	}
	if !h.Verify(hash, "correct horse") {
		t.Error("right password rejected")
	}
	if h.Verify(hash, "wrong horse") {
		t.Error("wrong password accepted")
	}
	again, _ := h.Hash("correct horse")
	if again == hash {
		t.Error("same password gave the same hash — salt missing")
	}
}

func TestHasher_UnknownUserTakesAsLongAsKnownUser(t *testing.T) {
	h := NewHasher(10) // slow enough to measure, fast enough for a test
	hash, _ := h.Hash("password1")

	start := time.Now()
	h.Verify(hash, "nope-nope")
	known := time.Since(start)

	start = time.Now()
	h.VerifyNobody("nope-nope")
	unknown := time.Since(start)

	if unknown < known/2 {
		t.Errorf("unknown email took %v vs %v for a known one — timing reveals which emails exist", unknown, known)
	}
}
