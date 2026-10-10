package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"relay/internal/app"
	"relay/internal/config"
	"relay/internal/testdb"
)

// newServer starts the whole API against a fresh database.
// configure (optional) adjusts the http.Server before it starts.
func newServer(t *testing.T, configure ...func(*http.Server)) *httptest.Server {
	t.Helper()
	pool := testdb.New(t)
	cfg := config.Config{
		JWTSecret:  []byte(strings.Repeat("k", 32)),
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
		BcryptCost: 4, // bcrypt.MinCost: tests don't need slow hashing
	}
	srv := httptest.NewUnstartedServer(app.New(cfg, pool, "test").Handler)
	for _, f := range configure {
		f(srv.Config)
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

// client is a tiny browser stand-in: it keeps cookies and an access token.
type client struct {
	t      *testing.T
	base   string
	http   *http.Client
	token  string
	UserID int64
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
}

// do sends a JSON request and decodes the JSON response into out (if non-nil).
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %q: %v", method, path, data, err)
		}
	}
	return res.StatusCode
}

type authResp struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
}

// signup registers `name` and keeps its access token.
func (c *client) signup(name string) *client {
	c.t.Helper()
	var out authResp
	code := c.do("POST", "/api/auth/signup", map[string]string{
		"email": name + "@example.com", "username": name, "display_name": strings.ToUpper(name[:1]) + name[1:],
		"password": "password123",
	}, &out)
	if code != http.StatusCreated {
		c.t.Fatalf("signup %s: status %d", name, code)
	}
	c.token, c.UserID = out.AccessToken, out.User.ID
	return c
}

type apiError struct {
	Error struct {
		Code   string            `json:"code"`
		Fields map[string]string `json:"fields"`
	} `json:"error"`
}

func stringsReader(s string) io.Reader { return strings.NewReader(s) }
