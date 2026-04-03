package command

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/payload"
	"go.resystems.io/renotify/internal/testutil"
)

func TestAnswer_Accepted(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	// Subscribe to capture the response.
	var received *nats.Msg
	sub, _ := nc.Subscribe(
		"resystems.renotify.testuser.flow.fl_TEST01.response",
		func(msg *nats.Msg) { received = msg })
	defer sub.Unsubscribe()

	stdout, _, err := executeCommand("answer",
		"-f", "fl_TEST01",
		"-n", "ntf_TEST01",
		"--accepted",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}

	// Verify JSON output.
	var resp payload.NotificationResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if resp.RequestID != "ntf_TEST01" {
		t.Errorf("request_id = %q, want %q", resp.RequestID, "ntf_TEST01")
	}
	if resp.Accepted == nil || !*resp.Accepted {
		t.Error("expected accepted=true")
	}

	// Verify the message arrived on NATS.
	nc.Flush()
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		return received != nil
	}) {
		t.Fatal("no message received on .response subject")
	}
	// Verify dedup header.
	if received.Header.Get("Nats-Msg-Id") != "ntf_TEST01-response" {
		t.Errorf("Nats-Msg-Id = %q", received.Header.Get("Nats-Msg-Id"))
	}
}

func TestAnswer_Rejected(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	stdout, _, err := executeCommand("answer",
		"-f", "fl_TEST02",
		"-n", "ntf_TEST02",
		"--rejected",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}

	var resp payload.NotificationResponse
	json.Unmarshal([]byte(stdout), &resp)
	if resp.Accepted == nil || *resp.Accepted {
		t.Error("expected accepted=false")
	}
}

func TestAnswer_ChoiceAction(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	stdout, _, err := executeCommand("answer",
		"-f", "fl_TEST03",
		"-n", "ntf_TEST03",
		"-a", "Approve",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}

	var resp payload.NotificationResponse
	json.Unmarshal([]byte(stdout), &resp)
	if resp.Action != "Approve" {
		t.Errorf("action = %q, want %q", resp.Action, "Approve")
	}
}

func TestAnswer_AcceptedWithText(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	stdout, _, err := executeCommand("answer",
		"-f", "fl_TEST04",
		"-n", "ntf_TEST04",
		"--accepted",
		"-m", "Looks good to me",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}

	var resp payload.NotificationResponse
	json.Unmarshal([]byte(stdout), &resp)
	if resp.Accepted == nil || !*resp.Accepted {
		t.Error("expected accepted=true")
	}
	if resp.Text != "Looks good to me" {
		t.Errorf("text = %q", resp.Text)
	}
}

func TestAnswer_TextSilent(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	stdout, _, err := executeCommand("answer",
		"-f", "fl_TEST05",
		"-n", "ntf_TEST05",
		"--accepted",
		"--format", "text",
	)
	if err != nil {
		t.Fatalf("answer failed: %v", err)
	}
	if stdout != "" {
		t.Errorf("text format should be silent, got %q", stdout)
	}
}

func TestAnswer_ValidationMissingFlowID(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("answer",
		"-n", "ntf_TEST",
		"--accepted",
	)
	if err == nil {
		t.Fatal("expected error for missing flow-id")
	}
	if !strings.Contains(err.Error(), "--flow-id") {
		t.Errorf("error = %q", err)
	}
}

func TestAnswer_ValidationMissingRequestID(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("answer",
		"-f", "fl_TEST",
		"--accepted",
	)
	if err == nil {
		t.Fatal("expected error for missing request-id")
	}
	if !strings.Contains(err.Error(), "--request-id") {
		t.Errorf("error = %q", err)
	}
}

func TestAnswer_ValidationAcceptedAndRejected(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("answer",
		"-f", "fl_TEST",
		"-n", "ntf_TEST",
		"--accepted",
		"--rejected",
	)
	if err == nil {
		t.Fatal("expected error for accepted+rejected")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q", err)
	}
}
