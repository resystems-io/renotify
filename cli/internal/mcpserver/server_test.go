package mcpserver

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"go.resystems.io/renotify/internal/httpserver"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestServer_Name(t *testing.T) {
	httpSrv := httpserver.New("127.0.0.1", 0, testLogger())
	s := New(httpSrv, testLogger(), "", "", nil)
	if s.Name() != "mcp" {
		t.Errorf("Name() = %q, want %q", s.Name(), "mcp")
	}
}

func TestServer_StartSignalsReady(t *testing.T) {
	httpSrv := httpserver.New("127.0.0.1", 0, testLogger())
	s := New(httpSrv, testLogger(), "", "", nil)

	ready := make(chan error, 1)
	err := s.Start(context.Background(), nil, ready)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("ready error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for ready")
	}
}

func TestServer_StopClean(t *testing.T) {
	httpSrv := httpserver.New("127.0.0.1", 0, testLogger())
	s := New(httpSrv, testLogger(), "", "", nil)

	ready := make(chan error, 1)
	s.Start(context.Background(), nil, ready)
	<-ready

	err := s.Stop(context.Background())
	if err != nil {
		t.Fatalf("Stop error: %v", err)
	}
}

func TestServer_RegistersHandler(t *testing.T) {
	httpSrv := httpserver.New("127.0.0.1", 0, testLogger())
	s := New(httpSrv, testLogger(), "", "", nil)

	// Start MCP (registers /mcp on httpSrv).
	mcpReady := make(chan error, 1)
	s.Start(context.Background(), nil, mcpReady)
	<-mcpReady

	// Start HTTP server to verify the handler is mounted.
	httpReady := make(chan error, 1)
	httpSrv.Start(context.Background(), nil, httpReady)
	<-httpReady
	defer httpSrv.Stop(context.Background())

	// MCP streamable HTTP handler rejects GET (expects POST).
	// A non-404 response confirms the handler is registered.
	resp, err := http.Get("http://" + httpSrv.Addr() + "/mcp")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == 404 {
		t.Error("GET /mcp returned 404 — handler not registered")
	}
}

func TestServer_HandlerReturnsNonNil(t *testing.T) {
	httpSrv := httpserver.New("127.0.0.1", 0, testLogger())
	s := New(httpSrv, testLogger(), "", "", nil)
	if s.Handler() == nil {
		t.Error("Handler() should return non-nil")
	}
}

func TestServer_SSEHandlerReturnsNonNil(t *testing.T) {
	httpSrv := httpserver.New("127.0.0.1", 0, testLogger())
	s := New(httpSrv, testLogger(), "", "", nil)
	if s.SSEHandler() == nil {
		t.Error("SSEHandler() should return non-nil")
	}
}

func TestServer_RegistersSSEHandler(t *testing.T) {
	httpSrv := httpserver.New("127.0.0.1", 0, testLogger())
	s := New(httpSrv, testLogger(), "", "", nil)

	mcpReady := make(chan error, 1)
	s.Start(context.Background(), nil, mcpReady)
	<-mcpReady

	httpReady := make(chan error, 1)
	httpSrv.Start(context.Background(), nil, httpReady)
	<-httpReady
	defer httpSrv.Stop(context.Background())

	// SSE handler accepts GET and holds the connection open.
	// Use a short timeout — a non-404 response (or timeout
	// from a held-open connection) proves the handler is mounted.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://" + httpSrv.Addr() + "/sse")
	if err != nil {
		// Timeout is expected — the SSE handler holds the GET
		// open to stream events. A timeout means the handler
		// accepted the request and started streaming.
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		t.Error("GET /sse returned 404 — SSE handler not registered")
	}
}
