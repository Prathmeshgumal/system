package users

import (
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"  Alice@Example.COM ", "alice@example.com", true},
		{"a@b", "a@b", true},
		{"no-at-sign", "", false},
		{"@example.com", "", false},
		{"alice@", "", false},
		{"", "", false},
		{strings.Repeat("a", 250) + "@b.co", "", false},
	}
	for _, c := range cases {
		got, msg := NormalizeEmail(c.in)
		if (msg == "") != c.ok || (c.ok && got != c.want) {
			t.Errorf("NormalizeEmail(%q) = %q, %q; want %q ok=%v", c.in, got, msg, c.want, c.ok)
		}
	}
}

func TestNormalizeUsername(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"Alice_99", "alice_99", true},
		{" bob ", "bob", true},
		{"ab", "", false},                    // too short
		{strings.Repeat("a", 33), "", false}, // too long
		{"al ice", "", false},                // space
		{"alïce", "", false},                 // non-ascii
		{"al-ice", "", false},                // dash
	}
	for _, c := range cases {
		got, msg := NormalizeUsername(c.in)
		if (msg == "") != c.ok || (c.ok && got != c.want) {
			t.Errorf("NormalizeUsername(%q) = %q, %q; want %q ok=%v", c.in, got, msg, c.want, c.ok)
		}
	}
}

func TestNormalizeDisplayName(t *testing.T) {
	if got, msg := NormalizeDisplayName("  Alice  "); msg != "" || got != "Alice" {
		t.Errorf("got %q, %q", got, msg)
	}
	if _, msg := NormalizeDisplayName("   "); msg == "" {
		t.Error("blank display name accepted")
	}
	if _, msg := NormalizeDisplayName(strings.Repeat("é", 65)); msg == "" {
		t.Error("65-character display name accepted")
	}
	if _, msg := NormalizeDisplayName(strings.Repeat("é", 64)); msg != "" {
		t.Error("64 characters (128 bytes) rejected — limits count characters, not bytes")
	}
}

func TestNormalizeBio(t *testing.T) {
	if _, msg := NormalizeBio(strings.Repeat("a", 281)); msg == "" {
		t.Error("281-character bio accepted")
	}
	if got, msg := NormalizeBio("  hi  "); msg != "" || got != "hi" {
		t.Errorf("got %q, %q", got, msg)
	}
}

func TestSearchPattern_EscapesLikeWildcards(t *testing.T) {
	// "_" is a LIKE wildcard: unescaped, "a_" would match "ab", "ac", ...
	if got := searchPattern("a_b"); got != `a\_b%` {
		t.Errorf("searchPattern = %q", got)
	}
}
