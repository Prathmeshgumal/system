package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbe_200_ReturnsNil(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := Probe(context.Background(), srv.URL); err != nil {
		t.Fatalf("Probe() = %v, want nil", err)
	}
}

func TestProbe_Non200_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if err := Probe(context.Background(), srv.URL); err == nil {
		t.Fatal("Probe() = nil, want error for 503")
	}
}

func TestProbe_ServerUnreachable_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing is listening on this address any more

	if err := Probe(context.Background(), url); err == nil {
		t.Fatal("Probe() = nil, want error for unreachable server")
	}
}

func TestProbe_ServerHangs_RespectsContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := Probe(ctx, srv.URL)
	if err == nil {
		t.Fatal("Probe() = nil, want timeout error")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Probe took %v, want it to stop at the 50ms deadline", elapsed)
	}
}
