// Package mcpserver provides a minimal MCP server that registers
// on the shared HTTP server. Tools are added by later items (C-06).
package mcpserver

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/httpserver"
)

// Server is a minimal MCP server implementing daemon.Subsystem.
type Server struct {
	httpSrv   *httpserver.Server
	logger    *slog.Logger
	mcpServer *mcp.Server
	handler   http.Handler
}

// New creates an MCP server that will register its handler on
// the shared HTTP server at /mcp.
func New(httpSrv *httpserver.Server, logger *slog.Logger) *Server {
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "renotify",
		Version: "0.1.0",
	}, nil)

	handler := mcp.NewStreamableHTTPHandler(
		func(req *http.Request) *mcp.Server {
			return mcpSrv
		}, nil)

	return &Server{
		httpSrv:   httpSrv,
		logger:    logger,
		mcpServer: mcpSrv,
		handler:   handler,
	}
}

// Name returns "mcp".
func (s *Server) Name() string { return "mcp" }

// Start registers the MCP SSE handler on the shared HTTP server.
// The nc parameter is accepted for future use by C-06 (tool
// registration) but unused in the minimal server.
func (s *Server) Start(_ context.Context, _ *nats.Conn, ready chan<- error) error {
	s.httpSrv.Handle("/mcp", s.handler)
	s.logger.Info("MCP server registered", "path", "/mcp")
	if ready != nil {
		close(ready)
	}
	return nil
}

// Stop is a no-op for the minimal MCP server — the HTTP server
// handles connection draining.
func (s *Server) Stop(_ context.Context) error {
	return nil
}

// Handler returns the MCP HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.handler
}
