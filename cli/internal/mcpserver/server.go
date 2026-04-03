// Package mcpserver implements the MCP server that exposes
// Renotify's capabilities to AI agents (C-06, C-11). Tools:
// register_flow, refresh_flow, terminate_flow, post, ask,
// await_decision, check_interjections, await_interjection.
// Resources: DecisionResource, InterjectionResource.
package mcpserver

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/httpserver"
	"go.resystems.io/renotify/internal/ledger"
)

// Server is the MCP server implementing daemon.Subsystem.
type Server struct {
	httpSrv    *httpserver.Server
	logger     *slog.Logger
	mcpServer  *mcp.Server
	handler    http.Handler
	sseHandler http.Handler

	// Dependencies set via constructor — available after New().
	db       func() *ledger.DB
	username string
	daemonID string
	cfg      *config.Config

	// Dependencies set in Start() — available after ready.
	nc *nats.Conn
	js nats.JetStreamContext

	// Decision tracking for async ask responses.
	decisions   *DecisionStore
	subscribers *SubscriberMap

	// Interjection tracking for mobile user signals.
	interjections *InterjectionStore

	cancelMu sync.Mutex

	// Stdio relay sessions (renotify mcp → NATS → mcp.Server).
	stdioMu       sync.Mutex
	stdioSessions map[string]*stdioSession
	stdioOpenSub  *nats.Subscription
	stdioCloseSub *nats.Subscription
}

// New creates an MCP server that will register its handler on
// the shared HTTP server at /mcp. Dependencies that require the
// NATS connection are bound in Start().
// New creates an MCP server. The dbFunc parameter is a lazy
// accessor for the ledger DB — it must return non-nil after
// the ledger subsystem has started. This allows construction
// before the ledger's Start() is called.
func New(
	httpSrv *httpserver.Server,
	logger *slog.Logger,
	dbFunc func() *ledger.DB,
	username, daemonID string,
	cfg *config.Config,
) *Server {
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    "renotify",
		Version: "0.1.0",
	}, &mcp.ServerOptions{
		Logger: logger,
		// Accept all resource subscriptions — the SDK manages the
		// subscription map internally. Required for
		// server.ResourceUpdated() to deliver notifications.
		SubscribeHandler: func(_ context.Context, _ *mcp.SubscribeRequest) error {
			return nil
		},
		UnsubscribeHandler: func(_ context.Context, _ *mcp.UnsubscribeRequest) error {
			return nil
		},
	})

	handler := mcp.NewStreamableHTTPHandler(
		func(req *http.Request) *mcp.Server {
			return mcpSrv
		}, nil)

	sseHandler := mcp.NewSSEHandler(
		func(req *http.Request) *mcp.Server {
			return mcpSrv
		}, nil)

	return &Server{
		httpSrv:       httpSrv,
		logger:        logger,
		mcpServer:     mcpSrv,
		handler:       handler,
		sseHandler:    sseHandler,
		db:            dbFunc,
		username:      username,
		daemonID:      daemonID,
		cfg:           cfg,
		decisions:     NewDecisionStore(),
		subscribers:   NewSubscriberMap(),
		interjections: NewInterjectionStore(),
	}
}

// Name returns "mcp".
func (s *Server) Name() string { return "mcp" }

// Start registers tools, resources, and the HTTP handler.
// If nc is nil, only the HTTP handler is registered (no tools).
func (s *Server) Start(_ context.Context, nc *nats.Conn, ready chan<- error) error {
	s.nc = nc

	if nc != nil {
		js, err := nc.JetStream()
		if err != nil {
			if ready != nil {
				ready <- err
				close(ready)
			}
			return err
		}
		s.js = js

		// Register tools and resources.
		s.registerFlowTools()
		s.registerPostTool()
		s.registerAskTool()
		s.registerAwaitDecisionTool()
		s.registerInterjectionTools()
		s.registerDecisionTemplate()
		s.registerInterjectionTemplate()

		// Start interjection consumer (C-11). Skipped when db is
		// nil (lightweight test mode without ledger).
		if s.db != nil {
			if err := s.startInterjectConsumer(context.Background()); err != nil {
				if ready != nil {
					ready <- err
					close(ready)
				}
				return err
			}
		}

		// Start stdio relay listener for `renotify mcp` sessions.
		// Skipped when username is empty (lightweight test mode).
		if s.username != "" {
			if err := s.startStdioRelay(context.Background()); err != nil {
				if ready != nil {
					ready <- err
					close(ready)
				}
				return err
			}
		}

		s.logger.Info("MCP tools registered",
			"tools", "register_flow, refresh_flow, terminate_flow, post, ask, await_decision, check_interjections, await_interjection")
	}

	s.httpSrv.Handle("/mcp", s.handler)
	s.httpSrv.Handle("/sse", s.sseHandler)
	s.logger.Info("MCP server registered",
		"streamable_path", "/mcp", "sse_path", "/sse")

	if ready != nil {
		close(ready)
	}
	return nil
}

// Stop cancels all pending response subscribers and stdio
// relay sessions.
func (s *Server) Stop(_ context.Context) error {
	s.subscribers.CancelAll()
	s.stopStdioRelay()
	s.logger.Info("MCP server stopped")
	return nil
}

// Handler returns the Streamable HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// SSEHandler returns the Standard SSE handler for testing.
func (s *Server) SSEHandler() http.Handler {
	return s.sseHandler
}
