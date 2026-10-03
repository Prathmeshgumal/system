package httpserver

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestServe_ShutdownSequence checks decision K end to end on a real socket:
// readiness fails first, in-flight requests finish, then the port closes.
func TestServe_ShutdownSequence(t *testing.T) {
	const drain = 300 * time.Millisecond

	var srv *Server
	mux := http.NewServeMux()
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if srv.ShuttingDown() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(600 * time.Millisecond) // still running when shutdown starts
		_, _ = io.WriteString(w, "done")
	})
	srv = New(":0", mux, zap.NewNop(), drain, 5*time.Second)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ctx, ln) }()

	if code := get(t, base+"/readyz"); code != http.StatusOK {
		t.Fatalf("before shutdown: readyz = %d, want 200", code)
	}

	// Start a slow request, then begin shutting down while it's in flight.
	slow := make(chan string, 1)
	go func() {
		resp, err := http.Get(base + "/slow")
		if err != nil {
			slow <- "error: " + err.Error()
			return
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		slow <- string(b)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel() // like SIGTERM

	// During the drain the server still answers, but says "not ready".
	time.Sleep(drain / 3)
	if code := get(t, base+"/readyz"); code != http.StatusServiceUnavailable {
		t.Fatalf("during drain: readyz = %d, want 503", code)
	}

	if got := <-slow; got != "done" {
		t.Fatalf("in-flight request = %q, want it to complete with \"done\"", got)
	}
	select {
	case err := <-served:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after shutdown")
	}

	if _, err := net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond); err == nil {
		t.Fatal("port still accepting connections after shutdown")
	}
}

func get(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}
