// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/payload"
	"go.resystems.io/renotify/cli/internal/testutil"
)

func TestInterject_Stop(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	var received *nats.Msg
	sub, _ := nc.Subscribe(
		"resystems.renotify.testuser.flow.fl_INTEST01.interject",
		func(msg *nats.Msg) { received = msg })
	defer sub.Unsubscribe()

	stdout, _, err := executeCommand("interject",
		"-f", "fl_INTEST01",
		"stop",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("interject failed: %v", err)
	}

	// Verify JSON output.
	var interj payload.InterjectionCommand
	if err := json.Unmarshal([]byte(stdout), &interj); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if interj.FlowID != "fl_INTEST01" {
		t.Errorf("flow_id = %q", interj.FlowID)
	}
	if interj.Action != payload.InterjectionStop {
		t.Errorf("action = %q, want %q", interj.Action, payload.InterjectionStop)
	}

	// Verify message arrived on NATS.
	nc.Flush()
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		return received != nil
	}) {
		t.Fatal("no message received on .interject subject")
	}
}

func TestInterject_NoteWithContext(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	var received *nats.Msg
	sub, _ := nc.Subscribe(
		"resystems.renotify.testuser.flow.fl_INTEST02.interject",
		func(msg *nats.Msg) { received = msg })
	defer sub.Unsubscribe()

	_, _, err := executeCommand("interject",
		"-f", "fl_INTEST02",
		"note",
		"-m", "Staging is down",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("interject failed: %v", err)
	}

	nc.Flush()
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		return received != nil
	}) {
		t.Fatal("no message received")
	}

	var interj payload.InterjectionCommand
	json.Unmarshal(received.Data, &interj)
	if interj.Action != payload.InterjectionNote {
		t.Errorf("action = %q, want %q", interj.Action, payload.InterjectionNote)
	}
	if interj.Context != "Staging is down" {
		t.Errorf("context = %q", interj.Context)
	}
}

func TestInterject_TextSilent(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	stdout, _, err := executeCommand("interject",
		"-f", "fl_INTEST03",
		"stop",
		"--format", "text",
	)
	if err != nil {
		t.Fatalf("interject failed: %v", err)
	}
	if stdout != "" {
		t.Errorf("text format should be silent, got %q", stdout)
	}
}

func TestInterject_ValidationMissingFlowID(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("interject", "stop")
	if err == nil {
		t.Fatal("expected error for missing flow-id")
	}
	if !strings.Contains(err.Error(), "--flow-id") {
		t.Errorf("error = %q", err)
	}
}

func TestInterject_ValidationMissingAction(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("interject",
		"-f", "fl_TEST")
	if err == nil {
		t.Fatal("expected error for missing action")
	}
}

func TestInterject_ValidationInvalidAction(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("interject",
		"-f", "fl_TEST",
		"invalid",
	)
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
	if !strings.Contains(err.Error(), "invalid action") {
		t.Errorf("error = %q", err)
	}
}

func TestInterject_Pause(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	stdout, _, err := executeCommand("interject",
		"-f", "fl_INTEST04",
		"pause",
		"-m", "Waiting for review",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("interject failed: %v", err)
	}

	var interj payload.InterjectionCommand
	json.Unmarshal([]byte(stdout), &interj)
	if interj.Action != payload.InterjectionPause {
		t.Errorf("action = %q, want %q", interj.Action, payload.InterjectionPause)
	}
	if interj.Context != "Waiting for review" {
		t.Errorf("context = %q", interj.Context)
	}
}
