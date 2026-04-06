// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	nats_server "github.com/nats-io/nats-server/v2/server"
	nats_test "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/testutil"
)

func TestPost_Integration(t *testing.T) {
	// Start an embedded NATS server with JetStream.
	opts := nats_test.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	srv := nats_test.RunServer(&opts)
	defer srv.Shutdown()

	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("RENOTIFY_BROKER_TCP_HOST", "127.0.0.1")
	t.Setenv("RENOTIFY_BROKER_TCP_PORT",
		strconv.Itoa(serverPort(srv)))

	// Write a fake internal token (the test NATS server has no
	// auth, so any token works — ConnectCLI reads it but the
	// server doesn't verify).
	tokenDir := filepath.Join(stateDir, "renotify")
	os.MkdirAll(tokenDir, 0700)
	os.WriteFile(filepath.Join(tokenDir, "internal_token"),
		[]byte("rn_tk_TESTTOKEN\n"), 0600)

	// Connect a subscriber to verify the message arrives.
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	// Create the RENOTIFY stream so JetStream publish succeeds.
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "RENOTIFY",
		Subjects: []string{"resystems.renotify.*.flow.>"},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe to all flow subjects to capture messages.
	var received []*nats.Msg
	sub, err := nc.Subscribe("resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			received = append(received, msg)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	// Run the post command.
	stdout, _, cmdErr := executeCommand("post",
		"--title", "Build done",
		"--message", "All tests passed",
		"--priority", "high",
		"--source", "ci/test",
		"--format", "json",
	)
	if cmdErr != nil {
		t.Fatalf("post command failed: %v", cmdErr)
	}

	// Verify JSON output.
	var output struct {
		Status         string `json:"status"`
		NotificationID string `json:"notification_id"`
	}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if output.Status != "sent" {
		t.Errorf("status = %q, want %q", output.Status, "sent")
	}
	if !strings.HasPrefix(output.NotificationID, "ntf_") {
		t.Errorf("notification_id should start with ntf_, got %q",
			output.NotificationID)
	}

	// Wait for messages to arrive.
	nc.Flush()
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		return len(received) >= 3
	}) {
		t.Fatalf("received %d messages, want at least 3", len(received))
	}

	// Verify subjects.
	var hasRequest, hasLifecycle bool
	for _, msg := range received {
		if strings.HasSuffix(msg.Subject, ".request") {
			hasRequest = true
			// Verify dedup header.
			msgID := msg.Header.Get("Nats-Msg-Id")
			if msgID == "" {
				t.Error("request message missing Nats-Msg-Id header")
			}
		}
		if strings.HasSuffix(msg.Subject, ".lifecycle") {
			hasLifecycle = true
		}
	}
	if !hasRequest {
		t.Error("no .request message received")
	}
	if !hasLifecycle {
		t.Error("no .lifecycle message received")
	}
}

func TestPost_TextOutputSilent(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	stdout, _, err := executeCommand("post",
		"--title", "Silent test",
		"--format", "text",
	)
	if err != nil {
		t.Fatalf("post command failed: %v", err)
	}
	if stdout != "" {
		t.Errorf("text format should produce no stdout, got %q", stdout)
	}
}

func TestPost_InvalidPriority(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("post",
		"--title", "Test",
		"--priority", "urgent",
	)
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
	if !strings.Contains(err.Error(), "invalid priority") {
		t.Errorf("error = %q, expected 'invalid priority'", err)
	}
}

// startTestNATS starts an embedded NATS server with JetStream
// and the RENOTIFY stream.
func startTestNATS(t *testing.T) (*nats_server.Server, string) {
	t.Helper()
	opts := nats_test.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	srv := nats_test.RunServer(&opts)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()
	js, _ := nc.JetStream()
	js.AddStream(&nats.StreamConfig{
		Name:     "RENOTIFY",
		Subjects: []string{"resystems.renotify.*.flow.>"},
		Storage:  nats.MemoryStorage,
	})

	stateDir := t.TempDir()
	tokenDir := filepath.Join(stateDir, "renotify")
	os.MkdirAll(tokenDir, 0700)
	os.WriteFile(filepath.Join(tokenDir, "internal_token"),
		[]byte("rn_tk_TESTTOKEN\n"), 0600)

	return srv, stateDir
}

func setupPostEnv(t *testing.T, srv *nats_server.Server, stateDir string) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", stateDir)
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("RENOTIFY_BROKER_TCP_HOST", "127.0.0.1")
	t.Setenv("RENOTIFY_BROKER_TCP_PORT",
		strconv.Itoa(serverPort(srv)))
}

// serverPort extracts the listening port from a NATS server.
func serverPort(srv *nats_server.Server) int {
	addr := srv.Addr()
	_, portStr, _ := net.SplitHostPort(addr.String())
	port, _ := strconv.Atoi(portStr)
	return port
}
