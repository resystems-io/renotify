// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/config"
	"go.resystems.io/renotify/cli/internal/httpserver"
	"go.resystems.io/renotify/cli/internal/mcpserver"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startPlainNATS starts a NATS server with JetStream for the
// stdio relay tests (same pattern as tool_test.go).
func startPlainNATS(t *testing.T) (*natsserver.Server, *nats.Conn) {
	t.Helper()
	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats not ready")
	}
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)

	// Create stream and consumers so the MCP server's
	// interjection consumer can bind.
	logger := discardLogger()
	if err := broker.EnsureJetStream(
		t.Context(), nc, testUsername, nil,
		config.Default().JetStream, logger,
	); err != nil {
		t.Fatal(err)
	}

	return srv, nc
}

// startMCPWithNATS creates an MCP server wired to a test NATS
// server, returning the server and NATS connection. The MCP
// server has tools registered and the stdio relay listener
// running.
func startMCPWithNATS(t *testing.T) (*mcpserver.Server, *nats.Conn) {
	t.Helper()
	_, nc := startPlainNATS(t)

	dir := t.TempDir()
	cfg := config.Default()
	cfg.Username = testUsername

	httpSrv := httpserver.New("127.0.0.1", 0, discardLogger())
	srv := mcpserver.New(httpSrv, discardLogger(),
		testUsername, testDaemonID, cfg)

	ready := make(chan error, 1)
	if err := srv.Start(context.Background(), nc, ready); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for MCP ready")
	}
	t.Cleanup(func() { srv.Stop(context.Background()) })

	_ = dir
	return srv, nc
}

func TestStdioRelay_SessionOpenClose(t *testing.T) {
	_, nc := startMCPWithNATS(t)

	sessionID := "ms_TESTOPEN01"

	// Subscribe to s2c to verify daemon responds.
	s2cSubj := broker.MCPServerToClientSubject(
		testUsername, sessionID)
	s2cSub, err := nc.Subscribe(s2cSubj, func(msg *nats.Msg) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s2cSub.Unsubscribe()

	// Open session.
	openPayload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	openSubj := broker.MCPSessionOpenSubject(testUsername)
	if err := nc.Publish(openSubj, openPayload); err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	// Give the daemon a moment to process the open.
	time.Sleep(100 * time.Millisecond)

	// Send an initialize request via c2s.
	c2sSubj := broker.MCPClientToServerSubject(
		testUsername, sessionID)
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	if err := nc.Publish(c2sSubj, []byte(initReq)); err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	// Read response from s2c.
	s2cSub.Unsubscribe()
	respSub, err := nc.SubscribeSync(s2cSubj)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := respSub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("no response on s2c: %v", err)
	}

	// Verify it's a valid JSON-RPC response with server info.
	var resp struct {
		ID     int `json:"id"`
		Result struct {
			ServerInfo struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal response: %v (data: %s)", err, msg.Data)
	}
	if resp.ID != 1 {
		t.Errorf("response id = %d, want 1", resp.ID)
	}
	if resp.Result.ServerInfo.Name != "renotify" {
		t.Errorf("serverInfo.name = %q, want %q",
			resp.Result.ServerInfo.Name, "renotify")
	}

	// Close session.
	closePayload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	closeSubj := broker.MCPSessionCloseSubject(testUsername)
	if err := nc.Publish(closeSubj, closePayload); err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	// Give the daemon a moment to process the close.
	time.Sleep(100 * time.Millisecond)
}

func TestStdioRelay_ToolsList(t *testing.T) {
	_, nc := startMCPWithNATS(t)

	sessionID := "ms_TESTTOOLS01"

	// Open session.
	openPayload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	if err := nc.Publish(
		broker.MCPSessionOpenSubject(testUsername),
		openPayload,
	); err != nil {
		t.Fatal(err)
	}
	nc.Flush()
	time.Sleep(100 * time.Millisecond)

	c2sSubj := broker.MCPClientToServerSubject(
		testUsername, sessionID)
	s2cSubj := broker.MCPServerToClientSubject(
		testUsername, sessionID)

	respSub, err := nc.SubscribeSync(s2cSubj)
	if err != nil {
		t.Fatal(err)
	}

	// Send initialize.
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	nc.Publish(c2sSubj, []byte(initReq))
	nc.Flush()

	// Read initialize response.
	if _, err := respSub.NextMsg(5 * time.Second); err != nil {
		t.Fatalf("no initialize response: %v", err)
	}

	// Send initialized notification.
	initdNotif := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	nc.Publish(c2sSubj, []byte(initdNotif))
	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	// Send tools/list.
	toolsReq := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	nc.Publish(c2sSubj, []byte(toolsReq))
	nc.Flush()

	msg, err := respSub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("no tools/list response: %v", err)
	}

	var resp struct {
		ID     int `json:"id"`
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("unmarshal: %v (data: %s)", err, msg.Data)
	}
	if resp.ID != 2 {
		t.Errorf("response id = %d, want 2", resp.ID)
	}

	// Verify expected tools are present.
	toolNames := make(map[string]bool)
	for _, tool := range resp.Result.Tools {
		toolNames[tool.Name] = true
	}
	for _, want := range []string{
		"register_flow", "post", "ask", "terminate_flow",
	} {
		if !toolNames[want] {
			t.Errorf("tools/list missing %q", want)
		}
	}

	// Clean up.
	closePayload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	nc.Publish(broker.MCPSessionCloseSubject(testUsername),
		closePayload)
	nc.Flush()
}

func TestStdioRelay_DuplicateOpen(t *testing.T) {
	_, nc := startMCPWithNATS(t)

	sessionID := "ms_TESTDUP01"
	openPayload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	openSubj := broker.MCPSessionOpenSubject(testUsername)

	// Open twice — second should be a no-op (logged warning).
	nc.Publish(openSubj, openPayload)
	nc.Flush()
	time.Sleep(100 * time.Millisecond)

	nc.Publish(openSubj, openPayload)
	nc.Flush()
	time.Sleep(100 * time.Millisecond)

	// Clean up.
	closePayload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})
	nc.Publish(broker.MCPSessionCloseSubject(testUsername),
		closePayload)
	nc.Flush()
}

func TestStdioRelay_InvalidPayload(t *testing.T) {
	_, nc := startMCPWithNATS(t)

	// Open with invalid JSON — should not crash.
	openSubj := broker.MCPSessionOpenSubject(testUsername)
	nc.Publish(openSubj, []byte("not json"))
	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	// Open with empty session_id — should not crash.
	nc.Publish(openSubj, []byte(`{"session_id":""}`))
	nc.Flush()
	time.Sleep(50 * time.Millisecond)
}
