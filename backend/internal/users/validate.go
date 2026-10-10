package users

import (
	"strings"
	"unicode/utf8"
)

// Each Normalize* returns the cleaned value, or a message saying what's wrong.
// Limits count characters (runes), not bytes: "é" is 2 bytes but 1 character.

func NormalizeEmail(s string) (string, string) {
	s = strings.ToLower(strings.TrimSpace(s))
	local, domain, ok := strings.Cut(s, "@")
	switch {
	case !ok || local == "" || domain == "":
		return "", "must look like name@example.com"
	case len(s) > 254: // the longest address SMTP allows
		return "", "must be at most 254 characters"
	}
	return s, ""
}

func NormalizeUsername(s string) (string, string) {
	s = strings.ToLower(strings.TrimSpace(s))
	if n := len(s); n < 3 || n > 32 {
		return "", "must be 3–32 characters"
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return "", "may only contain letters, digits and _"
		}
	}
	return s, ""
}

func NormalizeDisplayName(s string) (string, string) {
	s = strings.TrimSpace(s)
	if n := utf8.RuneCountInString(s); n < 1 || n > 64 {
		return "", "must be 1–64 characters"
	}
	return s, ""
}

func NormalizeBio(s string) (string, string) {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > 280 {
		return "", "must be at most 280 characters"
	}
	return s, ""
}

// searchPattern turns a username prefix into a LIKE pattern.
// "_" and "%" are LIKE wildcards, so they're escaped first.
func searchPattern(prefix string) string {
	r := strings.NewReplacer(`\`, `\\`, `_`, `\_`, `%`, `\%`)
	return r.Replace(prefix) + "%"
}
