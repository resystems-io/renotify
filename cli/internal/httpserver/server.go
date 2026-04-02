// Package httpserver provides a shared loopback HTTP server for
// the daemon. Subsystems register handlers on named paths (e.g.,
// /mcp for MCP SSE, /dashboard for future UI). The server binds
// to 127.0.0.1 (loopback only, plain HTTP).
package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// Server is a shared loopback HTTP server that implements the
// daemon.Subsystem interface.
type Server struct {
	host   string
	port   int
	logger *slog.Logger
	mux    *http.ServeMux
	srv    *http.Server
	ln     net.Listener

	mu   sync.Mutex
	addr string
}

// New creates a shared HTTP server. Call Handle() to register
// handlers before Start().
func New(host string, port int, logger *slog.Logger) *Server {
	return &Server{
		host:   host,
		port:   port,
		logger: logger,
		mux:    http.NewServeMux(),
	}
}

// Name returns "http".
func (s *Server) Name() string { return "http" }

// Handle registers a handler on the shared mux. Must be called
// before Start.
func (s *Server) Handle(pattern string, handler http.Handler) {
	s.mux.Handle(pattern, handler)
}

// Start begins listening. It closes ready on success or sends
// an error and closes on failure.
func (s *Server) Start(_ context.Context, _ *nats.Conn, ready chan<- error) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if ready != nil {
			ready <- fmt.Errorf("listen %s: %w", addr, err)
			close(ready)
		}
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s.ln = ln

	s.mu.Lock()
	s.addr = ln.Addr().String()
	s.mu.Unlock()

	s.srv = &http.Server{
		Handler: logMiddleware(s.logger, s.mux),
	}

	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("HTTP server error", "err", err)
		}
	}()

	s.logger.Info("HTTP server started", "addr", s.Addr())
	if ready != nil {
		close(ready)
	}
	return nil
}

// Stop gracefully shuts down the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	if s.srv != nil {
		s.logger.Info("HTTP server stopping")
		return s.srv.Shutdown(ctx)
	}
	return nil
}

// Addr returns the listener address after Start. Useful for tests
// that use port 0.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// logMiddleware wraps an http.Handler with request logging.
func logMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
		)
	})
}

// statusWriter captures the HTTP status code for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher so that SSE events are pushed
// to the client immediately through the logging middleware.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
