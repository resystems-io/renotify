package command

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/payload"
)

// --- V-00 Integration Smoke Tests ---
//
// These tests exercise the CLI post/ask/answer/interject
// commands end-to-end through an embedded NATS broker with a
// mock mobile subscriber bound to the durable JetStream
// consumer. See docs/renotify-refinements.md V-00.

// TestSmoke_PostRoundTrip verifies that `renotify post`
// publishes a NotificationRequest that arrives at the mobile
// consumer's deliver subject with correct payload fields.
func TestSmoke_PostRoundTrip(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc := connectAndSetupConsumers(t, srv)
	defer nc.Close()

	// Subscribe to the mobile push deliver subject.
	msgs := subscribeFlowWildcard(t, nc)

	// Run post command.
	stdout, _, err := executeCommand("post",
		"-t", "Build done",
		"-b", "All tests passed",
		"--priority", "high",
		"--source", "ci/pipeline",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}

	// Verify JSON output has notification_id.
	var postOut struct {
		Status         string `json:"status"`
		NotificationID string `json:"notification_id"`
	}
	if err := json.Unmarshal([]byte(stdout), &postOut); err != nil {
		t.Fatalf("invalid post output: %v", err)
	}
	if postOut.Status != "sent" {
		t.Errorf("status = %q, want %q", postOut.Status, "sent")
	}

	// Collect messages (request + lifecycle events).
	received := collectMessages(t, msgs, 3, 2*time.Second)

	// Find the .request message.
	var req payload.NotificationRequest
	found := false
	for _, msg := range received {
		if strings.HasSuffix(msg.Subject, ".request") {
			if err := json.Unmarshal(msg.Data, &req); err != nil {
				t.Fatalf("unmarshal request: %v", err)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no .request message received by mobile consumer")
	}

	// Verify payload fields.
	if req.Title != "Build done" {
		t.Errorf("title = %q", req.Title)
	}
	if req.Body != "All tests passed" {
		t.Errorf("body = %q", req.Body)
	}
	if req.Priority != payload.PriorityHigh {
		t.Errorf("priority = %q", req.Priority)
	}
	if req.Source != "ci/pipeline" {
		t.Errorf("source = %q", req.Source)
	}
	if len(req.ResponseTypes) != 1 ||
		req.ResponseTypes[0] != payload.ResponseNone {
		t.Errorf("response_types = %v", req.ResponseTypes)
	}
	if !strings.HasPrefix(req.ID, "ntf_") {
		t.Errorf("id = %q, want ntf_ prefix", req.ID)
	}
	if !strings.HasPrefix(req.FlowID, "fl_") {
		t.Errorf("flow_id = %q, want fl_ prefix", req.FlowID)
	}

	// Verify lifecycle events arrived.
	lifecycleCount := 0
	for _, msg := range received {
		if strings.HasSuffix(msg.Subject, ".lifecycle") {
			lifecycleCount++
		}
	}
	if lifecycleCount < 2 {
		t.Errorf("lifecycle events = %d, want >= 2 (active + completed)",
			lifecycleCount)
	}
}

// TestSmoke_AskWithMockResponse verifies the full ask round-trip:
// CLI publishes request, mock subscriber responds, CLI exits 0
// with the response.
func TestSmoke_AskWithMockResponse(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "2s")

	nc := connectAndSetupConsumers(t, srv)
	defer nc.Close()

	// Mock responder: watch for .request, publish response.
	nc.Subscribe("resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			if !strings.HasSuffix(msg.Subject, ".request") {
				return
			}
			var req payload.NotificationRequest
			json.Unmarshal(msg.Data, &req)

			go func() {
				time.Sleep(100 * time.Millisecond)
				respSubj := strings.TrimSuffix(
					msg.Subject, ".request") + ".response"
				accepted := true
				resp := payload.NotificationResponse{
					RequestID: req.ID,
					Accepted:  &accepted,
					Timestamp: time.Now().UTC(),
				}
				data, _ := json.Marshal(resp)
				respMsg := &nats.Msg{
					Subject: respSubj,
					Data:    data,
					Header:  nats.Header{},
				}
				respMsg.Header.Set("Nats-Msg-Id",
					req.ID+"-resp")
				js, _ := nc.JetStream()
				js.PublishMsg(respMsg)
			}()
		})

	stdout, _, err := executeCommand("ask",
		"-t", "Deploy?",
		"-r", "boolean",
		"--timeout", "5s",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("ask failed: %v", err)
	}

	var resp payload.NotificationResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("invalid ask output: %v\n%s", err, stdout)
	}
	if resp.Accepted == nil || !*resp.Accepted {
		t.Error("expected accepted=true")
	}
	if !strings.HasPrefix(resp.RequestID, "ntf_") {
		t.Errorf("request_id = %q", resp.RequestID)
	}
}

// TestSmoke_AskWithAnswerUtility verifies that `renotify answer`
// can unblock a waiting `renotify ask`.
func TestSmoke_AskWithAnswerUtility(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "2s")

	nc := connectAndSetupConsumers(t, srv)
	defer nc.Close()

	// Capture the request to extract flow_id and notification_id
	// (needed while ask is still running).
	reqCh := make(chan payload.NotificationRequest, 1)
	nc.Subscribe("resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			if strings.HasSuffix(msg.Subject, ".request") {
				var req payload.NotificationRequest
				json.Unmarshal(msg.Data, &req)
				select {
				case reqCh <- req:
				default:
				}
			}
		})

	// Run ask in background.
	stdoutCh, _, errCh := runAskInBackground(t,
		"-t", "Approve?", "-r", "boolean", "--timeout", "10s")

	// Wait for the request to arrive on NATS.
	var req payload.NotificationRequest
	select {
	case req = <-reqCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ask to publish request")
	}

	// Run answer to respond.
	_, _, ansErr := executeCommand("answer",
		"-f", req.FlowID, "-n", req.ID, "--accepted")
	if ansErr != nil {
		t.Fatalf("answer failed: %v", ansErr)
	}

	// Wait for ask to complete.
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("ask failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ask did not exit after answer")
	}

	stdout := <-stdoutCh
	var resp payload.NotificationResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("invalid ask output: %v\n%s", err, stdout)
	}
	if resp.Accepted == nil || !*resp.Accepted {
		t.Error("expected accepted=true from answer")
	}
}

// TestSmoke_AskWithInterjectStop verifies that
// `renotify interject stop` terminates a waiting `renotify ask`.
func TestSmoke_AskWithInterjectStop(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "2s")

	nc := connectAndSetupConsumers(t, srv)
	defer nc.Close()

	// Capture the request to extract flow_id.
	reqCh := make(chan payload.NotificationRequest, 1)
	nc.Subscribe("resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			if strings.HasSuffix(msg.Subject, ".request") {
				var req payload.NotificationRequest
				json.Unmarshal(msg.Data, &req)
				select {
				case reqCh <- req:
				default:
				}
			}
		})

	// Run ask in background.
	_, _, errCh := runAskInBackground(t,
		"-t", "Proceed?", "-r", "boolean", "--timeout", "10s")

	// Wait for the request to arrive on NATS.
	var req payload.NotificationRequest
	select {
	case req = <-reqCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ask to publish request")
	}

	// Send stop interjection.
	_, _, intErr := executeCommand("interject",
		"-f", req.FlowID, "stop")
	if intErr != nil {
		t.Fatalf("interject failed: %v", intErr)
	}

	// Wait for ask to exit with error.
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected ask to fail after stop interjection")
		}
		if !strings.Contains(err.Error(), "stopped by user") {
			t.Errorf("error = %q, expected 'stopped by user'",
				err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ask did not exit after interject stop")
	}
}

// TestSmoke_JetStreamBuffering verifies that messages published
// before the mobile subscriber connects are delivered when the
// subscriber binds to the durable consumer.
func TestSmoke_JetStreamBuffering(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	// Create consumers first (daemon does this at startup).
	nc1 := connectAndSetupConsumers(t, srv)
	nc1.Close()

	// Publish post BEFORE mobile subscriber connects.
	_, _, err := executeCommand("post",
		"-t", "Buffered message",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}

	// NOW connect and subscribe to the mobile push deliver
	// subject. The durable consumer has buffered the messages
	// and will deliver them when we subscribe.
	nc2, err2 := nats.Connect(srv.ClientURL())
	if err2 != nil {
		t.Fatal(err2)
	}
	defer nc2.Close()

	deliverSubject :=
		"resystems.renotify.testuser.mobile.deliver"
	sub, err2 := nc2.SubscribeSync(deliverSubject)
	if err2 != nil {
		t.Fatal(err2)
	}
	defer sub.Unsubscribe()

	// Collect delivered messages and look for the request.
	foundRequest := false
	for range 10 {
		msg, err := sub.NextMsg(2 * time.Second)
		if err != nil {
			break
		}
		var req payload.NotificationRequest
		if json.Unmarshal(msg.Data, &req) == nil &&
			req.Title == "Buffered message" {
			foundRequest = true
			msg.Ack()
			break
		}
		msg.Ack()
	}
	if !foundRequest {
		t.Fatal("buffered message not delivered to mobile consumer")
	}
}

// TestSmoke_AskSafetyTimeout verifies that the CLI safety timer
// fires when no response is received (daemon timeout not yet
// implemented).
func TestSmoke_AskSafetyTimeout(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "1s")

	connectAndSetupConsumers(t, srv)

	// No responder — safety timer should fire.
	_, _, err := executeCommand("ask",
		"-t", "Will timeout",
		"-r", "boolean",
		"--timeout", "1s",
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	ce, ok := err.(*exitcode.CodedError)
	if !ok {
		t.Fatalf("expected CodedError, got %T: %v", err, err)
	}
	if ce.Code != exitcode.Timeout {
		t.Errorf("exit code = %d, want %d", ce.Code, exitcode.Timeout)
	}
}

// TestSmoke_PayloadSerialisation verifies that the JSON wire
// format uses snake_case keys, correct types, and omitempty.
func TestSmoke_PayloadSerialisation(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc := connectAndSetupConsumers(t, srv)
	defer nc.Close()

	msgs := subscribeFlowWildcard(t, nc)

	// Small delay to ensure subscription is active before
	// publishing.
	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	_, _, err := executeCommand("post",
		"-t", "Serialisation test",
		"-b", "Check JSON format",
		"--priority", "low",
		"--source", "test/v00",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("post failed: %v", err)
	}

	received := collectMessages(t, msgs, 3, 2*time.Second)

	// Find the .request message and verify raw JSON.
	for _, msg := range received {
		if !strings.HasSuffix(msg.Subject, ".request") {
			continue
		}

		// Verify snake_case keys in raw JSON.
		raw := string(msg.Data)
		for _, key := range []string{
			`"id"`, `"flow_id"`, `"daemon_id"`,
			`"workspace_id"`, `"title"`, `"body"`,
			`"response_types"`, `"priority"`, `"source"`,
			`"timestamp"`,
		} {
			if !strings.Contains(raw, key) {
				t.Errorf("missing JSON key %s in %s", key, raw)
			}
		}

		// Verify omitempty: actions and timeout_sec should be
		// absent for post (no actions, timeout=0).
		if strings.Contains(raw, `"actions"`) {
			t.Error("actions should be omitted (omitempty)")
		}
		if strings.Contains(raw, `"timeout_sec"`) {
			t.Error("timeout_sec should be omitted (omitempty)")
		}

		// Verify response_types is ["none"].
		if !strings.Contains(raw, `["none"]`) {
			t.Errorf("response_types should be [\"none\"], got %s",
				raw)
		}

		// Verify timestamp is RFC 3339.
		var obj map[string]interface{}
		json.Unmarshal(msg.Data, &obj)
		ts, ok := obj["timestamp"].(string)
		if !ok {
			t.Fatal("timestamp missing or not a string")
		}
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("timestamp %q is not RFC 3339: %v", ts, err)
		}

		return
	}
	t.Fatal("no .request message received")
}

// --- Smoke test helpers ---

// connectAndSetupConsumers connects to the test NATS server and
// creates the durable JetStream consumers (same as the daemon
// would via broker.EnsureJetStream).
func connectAndSetupConsumers(
	t *testing.T, srv interface{ ClientURL() string },
) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default().JetStream
	logger := slog.New(slog.NewTextHandler(
		&bytes.Buffer{}, nil))

	if err := broker.EnsureJetStream(
		context.Background(), nc, "testuser", cfg, logger,
	); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { nc.Close() })
	return nc
}

// subscribeFlowWildcard subscribes to all flow subjects via
// Core NATS (not the JetStream consumer) and returns a channel
// of received messages. This preserves the original subject for
// suffix-based discrimination.
func subscribeFlowWildcard(
	t *testing.T, nc *nats.Conn,
) chan *nats.Msg {
	t.Helper()
	ch := make(chan *nats.Msg, 100)
	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) { ch <- msg })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sub.Unsubscribe() })
	return ch
}

// collectMessages drains up to n messages from ch within the
// timeout. Returns what was collected (may be fewer than n).
func collectMessages(
	t *testing.T, ch chan *nats.Msg,
	n int, timeout time.Duration,
) []*nats.Msg {
	t.Helper()
	var msgs []*nats.Msg
	deadline := time.After(timeout)
	for len(msgs) < n {
		select {
		case msg := <-ch:
			msgs = append(msgs, msg)
		case <-deadline:
			return msgs
		}
	}
	return msgs
}

// runAskInBackground runs `renotify ask` in a goroutine and
// returns channels for stdout, stderr, and error.
func runAskInBackground(
	t *testing.T, args ...string,
) (stdoutCh chan string, stderrCh chan string, errCh chan error) {
	t.Helper()
	stdoutCh = make(chan string, 1)
	stderrCh = make(chan string, 1)
	errCh = make(chan error, 1)

	fullArgs := append([]string{"ask", "--format", "json"}, args...)

	go func() {
		root := NewRoot()
		var outBuf, errBuf bytes.Buffer
		root.SetOut(&outBuf)
		root.SetErr(&errBuf)
		root.SetArgs(fullArgs)
		err := root.Execute()

		stderrCh <- errBuf.String()
		stdoutCh <- outBuf.String()
		errCh <- err
	}()

	return
}
