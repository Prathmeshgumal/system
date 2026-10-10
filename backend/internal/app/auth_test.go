package app_test

import (
	"net/http"
	"testing"
)

func TestAuth_SignupLoginRefreshLogout(t *testing.T) {
	srv := newServer(t)
	c := newClient(t, srv).signup("alice")

	// The access token works…
	var me struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if code := c.do("GET", "/api/me", nil, &me); code != 200 || me.Username != "alice" || me.Email != "alice@example.com" {
		t.Fatalf("GET /api/me = %d %+v", code, me)
	}

	// …and the refresh cookie gets a new one without a password.
	var ref authResp
	if code := c.do("POST", "/api/auth/refresh", nil, &ref); code != 200 || ref.AccessToken == "" || ref.User.Username != "alice" {
		t.Fatalf("refresh = %d %+v", code, ref)
	}

	// Logout ends the session: refreshing no longer works.
	if code := c.do("POST", "/api/auth/logout", nil, nil); code != http.StatusNoContent {
		t.Fatalf("logout = %d", code)
	}
	if code := c.do("POST", "/api/auth/refresh", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout = %d, want 401", code)
	}

	// Log in again with the password.
	var login authResp
	code := c.do("POST", "/api/auth/login", map[string]string{"email": "ALICE@example.com", "password": "password123"}, &login)
	if code != 200 || login.User.Username != "alice" {
		t.Fatalf("login = %d %+v", code, login)
	}
}

func TestAuth_LoginFailuresLookTheSame(t *testing.T) {
	srv := newServer(t)
	newClient(t, srv).signup("alice")
	c := newClient(t, srv)

	var wrongPw, noUser apiError
	c1 := c.do("POST", "/api/auth/login", map[string]string{"email": "alice@example.com", "password": "nope-nope"}, &wrongPw)
	c2 := c.do("POST", "/api/auth/login", map[string]string{"email": "bob@example.com", "password": "nope-nope"}, &noUser)
	if c1 != 401 || c2 != 401 || wrongPw.Error.Code != noUser.Error.Code {
		t.Errorf("wrong password = %d %s, unknown email = %d %s — must be identical",
			c1, wrongPw.Error.Code, c2, noUser.Error.Code)
	}
}

func TestAuth_SignupValidationAndConflicts(t *testing.T) {
	srv := newServer(t)
	newClient(t, srv).signup("alice")
	c := newClient(t, srv)

	var bad apiError
	code := c.do("POST", "/api/auth/signup", map[string]string{
		"email": "nope", "username": "a b", "display_name": "", "password": "short",
	}, &bad)
	if code != 400 || len(bad.Error.Fields) != 4 {
		t.Errorf("invalid signup = %d %+v, want 400 with 4 field errors", code, bad.Error)
	}

	var taken apiError
	code = c.do("POST", "/api/auth/signup", map[string]string{
		"email": "other@example.com", "username": "ALICE", "display_name": "x", "password": "password123",
	}, &taken)
	if code != 409 || taken.Error.Code != "username_taken" {
		t.Errorf("taken username = %d %s", code, taken.Error.Code)
	}
}

func TestAuth_ProtectedRoutesNeedToken(t *testing.T) {
	srv := newServer(t)
	c := newClient(t, srv)
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Errorf("no token = %d", code)
	}
	c.token = "garbage"
	if code := c.do("GET", "/api/me", nil, nil); code != 401 {
		t.Errorf("bad token = %d", code)
	}
	// Public routes stay public.
	if code := c.do("GET", "/api/status", nil, nil); code != 200 {
		t.Errorf("/api/status = %d", code)
	}
}

func TestAuth_RefreshRejectsCrossOrigin(t *testing.T) {
	srv := newServer(t)
	c := newClient(t, srv).signup("alice")
	req, _ := http.NewRequest("POST", srv.URL+"/api/auth/refresh", nil)
	req.Header.Set("Origin", "https://evil.example")
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-origin refresh = %d, want 403", res.StatusCode)
	}
}

func TestAuth_RefreshCookieFlags(t *testing.T) {
	srv := newServer(t)
	c := newClient(t, srv)
	req, _ := http.NewRequest("POST", srv.URL+"/api/auth/signup", stringsReader(
		`{"email":"a@x.io","username":"alice","display_name":"A","password":"password123"}`))
	res, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var ck *http.Cookie
	for _, k := range res.Cookies() {
		if k.Name == "relay_refresh" {
			ck = k
		}
	}
	if ck == nil || !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/api/auth" {
		t.Errorf("refresh cookie = %+v", ck)
	}
}

func TestUsers_ProfileAndSearch(t *testing.T) {
	srv := newServer(t)
	alice := newClient(t, srv).signup("alice")
	newClient(t, srv).signup("albert")
	bob := newClient(t, srv).signup("bob")

	var found struct {
		Users []struct {
			Username string `json:"username"`
			Email    string `json:"email"`
		} `json:"users"`
	}
	if code := bob.do("GET", "/api/users?q=AL", nil, &found); code != 200 || len(found.Users) != 2 {
		t.Fatalf("search = %d %+v", code, found)
	}
	if found.Users[0].Email != "" {
		t.Error("search leaks email addresses")
	}

	var me struct {
		Bio         string `json:"bio"`
		DisplayName string `json:"display_name"`
	}
	code := alice.do("PATCH", "/api/me", map[string]string{"bio": "hi there", "display_name": "Alice A."}, &me)
	if code != 200 || me.Bio != "hi there" || me.DisplayName != "Alice A." {
		t.Fatalf("PATCH /api/me = %d %+v", code, me)
	}
	var e apiError
	if code := alice.do("PATCH", "/api/me", map[string]string{"username": "bob"}, &e); code != 409 {
		t.Errorf("rename to taken = %d", code)
	}
}
