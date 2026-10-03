package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Server is an http.Server plus the full graceful shutdown sequence
// (knowledge-base/notes/phase-0-notes.md, note 12).
type Server struct {
	srv             *http.Server
	log             *zap.Logger
	drainDelay      time.Duration
	shutdownTimeout time.Duration
	shuttingDown    atomic.Bool
}

// New creates a server with explicit timeouts. Go's zero-value http.Server has
// none, so a slow client could hold a connection forever (slowloris).
func New(addr string, h http.Handler, log *zap.Logger, drainDelay, shutdownTimeout time.Duration) *Server {
	return &Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           h,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       15 * time.Second,
			WriteTimeout:      15 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
		log:             log,
		drainDelay:      drainDelay,
		shutdownTimeout: shutdownTimeout,
	}
}

// ShuttingDown reports whether shutdown has begun. /readyz uses it to tell the
// load balancer to stop sending new requests.
func (s *Server) ShuttingDown() bool { return s.shuttingDown.Load() }

// Run listens on the configured address and calls Serve.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.srv.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.srv.Addr, err)
	}
	return s.Serve(ctx, ln)
}

// Serve handles requests on ln until ctx is cancelled (SIGTERM / Ctrl+C), then:
//
//  1. marks the server as shutting down → /readyz returns 503
//  2. keeps serving for drainDelay, so the load balancer notices and stops sending
//  3. stops accepting and waits up to shutdownTimeout for in-flight requests
//
// Closing the DB pool and flushing logs (steps 4–5) are done by the caller after Serve returns.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	serveErr := make(chan error, 1)
	go func() {
		s.log.Info("api listening", zap.String("addr", ln.Addr().String()))
		serveErr <- s.srv.Serve(ln)
	}()

	select {
	case err := <-serveErr:
		return err // failed before any shutdown was requested
	case <-ctx.Done():
	}

	s.shuttingDown.Store(true)
	s.log.Info("shutdown started: readiness now failing", zap.Duration("drain_delay", s.drainDelay))
	time.Sleep(s.drainDelay)

	s.log.Info("draining in-flight requests", zap.Duration("timeout", s.shutdownTimeout))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	if err := s.srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	s.log.Info("http server stopped")
	return nil
}
