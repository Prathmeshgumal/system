package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	audienceAPI = "api" // access tokens: Authorization: Bearer …
	audienceWS  = "ws"  // WebSocket tickets: /ws?ticket=…
	ticketTTL   = 60 * time.Second
)

// Tokens issues and checks signed JWTs (HS256).
//
// A JWT is three base64url parts: header.payload.signature. Anyone can READ the
// payload — the signature only proves we wrote it. So: no secrets in claims.
type Tokens struct {
	secret    []byte
	accessTTL time.Duration
	now       func() time.Time // swapped in tests
}

func NewTokens(secret []byte, accessTTL time.Duration) *Tokens {
	return &Tokens{secret: secret, accessTTL: accessTTL, now: time.Now}
}

// Claims is what an access token tells us about the caller.
type Claims struct {
	UserID    int64
	SessionID string
}

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func (t *Tokens) IssueAccess(userID int64, sessionID string) (string, time.Time, error) {
	now := t.now()
	exp := now.Add(t.accessTTL)
	tok, err := t.sign(&accessClaims{
		SessionID:        sessionID,
		RegisteredClaims: t.registered(userID, audienceAPI, now, exp),
	})
	return tok, exp, err
}

func (t *Tokens) ParseAccess(token string) (Claims, error) {
	var c accessClaims
	if err := t.parse(token, audienceAPI, &c); err != nil {
		return Claims{}, err
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return Claims{}, errors.New("bad subject")
	}
	return Claims{UserID: id, SessionID: c.SessionID}, nil
}

// IssueTicket makes a 60-second token that only the WebSocket endpoint accepts.
// Browsers can't add an Authorization header to a WebSocket, so the ticket
// travels in the URL — short-lived, so a copy in a log is useless.
func (t *Tokens) IssueTicket(userID int64) (string, error) {
	now := t.now()
	return t.sign(t.registered(userID, audienceWS, now, now.Add(ticketTTL)))
}

func (t *Tokens) ParseTicket(token string) (int64, error) {
	var c jwt.RegisteredClaims
	if err := t.parse(token, audienceWS, &c); err != nil {
		return 0, err
	}
	return strconv.ParseInt(c.Subject, 10, 64)
}

func (t *Tokens) registered(userID int64, aud string, now, exp time.Time) jwt.RegisteredClaims {
	return jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Audience:  jwt.ClaimStrings{aud},
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}
}

func (t *Tokens) sign(claims jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(t.secret)
}

// parse checks signature, algorithm, audience and expiry.
// Pinning the algorithm to HS256 blocks the "alg: none" and algorithm-confusion attacks.
func (t *Tokens) parse(token, aud string, claims jwt.Claims) error {
	p := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithAudience(aud),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(t.now),
	)
	_, err := p.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return t.secret, nil })
	return err
}
