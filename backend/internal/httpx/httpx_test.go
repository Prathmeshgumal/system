package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandle_ErrorBecomesJSON(t *testing.T) {
	h := Handle(func(w http.ResponseWriter, r *http.Request) error {
		return Invalid(map[string]string{"email": "must contain @"})
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body struct {
		Error struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "invalid_input" || body.Error.Fields["email"] != "must contain @" {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestHandle_UnknownErrorIs500WithoutDetails(t *testing.T) {
	h := Handle(func(w http.ResponseWriter, r *http.Request) error {
		return errors.New("pq: password authentication failed for user secret")
	})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Error("internal error details leaked to the client")
	}
}

func TestReadJSON(t *testing.T) {
	type in struct {
		Name string `json:"name"`
	}
	cases := map[string]struct {
		body string
		ok   bool
	}{
		"valid":         {`{"name":"a"}`, true},
		"malformed":     {`{"name":`, false},
		"unknown field": {`{"name":"a","admin":true}`, false},
		"two objects":   {`{"name":"a"}{"name":"b"}`, false},
		"too large":     {`{"name":"` + strings.Repeat("a", 2<<20) + `"}`, false},
		"empty body":    {``, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", bytes.NewBufferString(c.body))
			var v in
			err := ReadJSON(httptest.NewRecorder(), r, &v)
			if (err == nil) != c.ok {
				t.Fatalf("err = %v, want ok=%v", err, c.ok)
			}
			var he *Error
			if err != nil && (!errors.As(err, &he) || he.Status < 400 || he.Status > 499) {
				t.Errorf("want *Error with a 4xx status, got %v", err)
			}
		})
	}
}

func TestRecover_PanicBecomes500(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestLog_RecordsStatusAndKeepsUnwrap(t *testing.T) {
	var sawUnwrap bool
	h := Log(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, sawUnwrap = w.(interface{ Unwrap() http.ResponseWriter })
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d", rec.Code)
	}
	if !sawUnwrap {
		t.Error("wrapped writer must expose Unwrap so WebSocket upgrades can hijack the connection")
	}
}
