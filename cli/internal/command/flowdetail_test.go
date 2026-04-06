package command

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/statesvc"
)

func TestFlow_RequiresArg(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("flow")
	if err == nil {
		t.Fatal("expected error for missing flow_id arg")
	}
}

func TestFlow_NotFound(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows",
		func(msg *nats.Msg) {
			msg.Respond([]byte(`{"flows":[]}`))
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	_, _, err = executeCommand("flow", "fl_NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for nonexistent flow")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err)
	}
}

func TestFlow_TextOutput(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	now := time.Now().UTC()
	entry := statesvc.FlowEntry{
		FlowID:      "fl_DETAIL01",
		DaemonID:    "dn_TEST1234ABCDE",
		WorkspaceID: "ws_TESTWS01",
		DisplayName: "myproject",
		AbsPath:     "/home/test/myproject",
		Label:       "Build Check",
		Metadata: map[string]string{
			"branch": "main",
			"task":   "testing",
		},
		RegisteredAt:          now.Add(-2 * time.Minute),
		LastActivityTimestamp: now.Add(-30 * time.Second),
	}

	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows",
		func(msg *nats.Msg) {
			result := statesvc.FlowsResult{
				Flows: []statesvc.FlowEntry{entry},
			}
			data, _ := json.Marshal(result)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	stdout, _, err := executeCommand("flow", "fl_DETAIL01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"fl_DETAIL01",
		"dn_TEST1234ABCDE",
		"ws_TESTWS01",
		"myproject",
		"/home/test/myproject",
		"Build Check",
		"branch",
		"main",
		"task",
		"testing",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q\n%s", want, stdout)
		}
	}
}

func TestFlow_JSONOutput(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	now := time.Now().UTC()
	entry := statesvc.FlowEntry{
		FlowID:      "fl_JSON01",
		DaemonID:    "dn_TEST1234ABCDE",
		WorkspaceID: "ws_TESTWS01",
		DisplayName: "proj",
		AbsPath:     "/home/test/proj",
		Label:       "Deploy",
		Metadata: map[string]string{
			"env": "staging",
		},
		RegisteredAt:          now,
		LastActivityTimestamp: now,
	}

	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows",
		func(msg *nats.Msg) {
			result := statesvc.FlowsResult{
				Flows: []statesvc.FlowEntry{entry},
			}
			data, _ := json.Marshal(result)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	stdout, _, err := executeCommand("flow", "--format", "json",
		"fl_JSON01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got statesvc.FlowEntry
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if got.FlowID != "fl_JSON01" {
		t.Errorf("flow_id = %q, want %q",
			got.FlowID, "fl_JSON01")
	}
	if got.AbsPath != "/home/test/proj" {
		t.Errorf("abs_path = %q, want %q",
			got.AbsPath, "/home/test/proj")
	}
	if got.Metadata["env"] != "staging" {
		t.Errorf("metadata[env] = %q, want %q",
			got.Metadata["env"], "staging")
	}
}

func TestFlow_NoMetadata(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	now := time.Now().UTC()
	entry := statesvc.FlowEntry{
		FlowID:                "fl_NOMETA01",
		DaemonID:              "dn_TEST1234ABCDE",
		WorkspaceID:           "ws_TESTWS01",
		RegisteredAt:          now,
		LastActivityTimestamp: now,
	}

	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows",
		func(msg *nats.Msg) {
			result := statesvc.FlowsResult{
				Flows: []statesvc.FlowEntry{entry},
			}
			data, _ := json.Marshal(result)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	stdout, _, err := executeCommand("flow", "fl_NOMETA01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not contain "Metadata:" section when empty.
	if strings.Contains(stdout, "Metadata:") {
		t.Error("should not show Metadata section when empty")
	}
	if !strings.Contains(stdout, "fl_NOMETA01") {
		t.Error("missing flow ID in output")
	}
}
