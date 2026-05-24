// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

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
	if !strings.Contains(err.Error(), "no active flows found") {
		t.Errorf("error = %q, want 'no active flows found'", err)
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

func TestFlow_SearchName(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	entry := statesvc.FlowEntry{
		FlowID:      "fl_SRCH01",
		DaemonID:    "dn_TEST1234",
		WorkspaceID: "ws_TESTWS01",
		DisplayName: "mysearchproj",
		AbsPath:     "/home/test/mysearchproj",
	}

	// Mock search endpoint
	subSearch, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows.search",
		func(msg *nats.Msg) {
			res := statesvc.SearchFlowsResult{FlowIDs: []string{"fl_SRCH01"}}
			data, _ := json.Marshal(res)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer subSearch.Unsubscribe()

	// Mock strict endpoint
	subStrict, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows",
		func(msg *nats.Msg) {
			result := statesvc.FlowsResult{Flows: []statesvc.FlowEntry{entry}}
			data, _ := json.Marshal(result)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer subStrict.Unsubscribe()

	stdout, _, err := executeCommand("flow", "mysearchproj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "fl_SRCH01") {
		t.Errorf("output missing flow ID\n%s", stdout)
	}
	if !strings.Contains(stdout, "mysearchproj") {
		t.Errorf("output missing workspace name\n%s", stdout)
	}
}

func TestFlow_SearchPath(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	entry := statesvc.FlowEntry{FlowID: "fl_SRCH02", AbsPath: "/absolute/path"}

	subSearch, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows.search",
		func(msg *nats.Msg) {
			res := statesvc.SearchFlowsResult{FlowIDs: []string{"fl_SRCH02"}}
			data, _ := json.Marshal(res)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer subSearch.Unsubscribe()

	subStrict, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows",
		func(msg *nats.Msg) {
			result := statesvc.FlowsResult{Flows: []statesvc.FlowEntry{entry}}
			data, _ := json.Marshal(result)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer subStrict.Unsubscribe()

	stdout, _, err := executeCommand("flow", "/absolute/path")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "fl_SRCH02") {
		t.Errorf("output missing flow ID\n%s", stdout)
	}
}

func TestFlow_MultipleFlowsSeparator(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	e1 := statesvc.FlowEntry{FlowID: "fl_M1"}
	e2 := statesvc.FlowEntry{FlowID: "fl_M2"}

	subSearch, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows.search",
		func(msg *nats.Msg) {
			res := statesvc.SearchFlowsResult{FlowIDs: []string{"fl_M1", "fl_M2"}}
			data, _ := json.Marshal(res)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer subSearch.Unsubscribe()

	subStrict, err := nc.Subscribe(
		"resystems.renotify.testuser.svc.flows",
		func(msg *nats.Msg) {
			var query statesvc.FlowsQuery
			json.Unmarshal(msg.Data, &query)
			var result statesvc.FlowsResult
			if query.FlowID == "fl_M1" {
				result = statesvc.FlowsResult{Flows: []statesvc.FlowEntry{e1}}
			} else {
				result = statesvc.FlowsResult{Flows: []statesvc.FlowEntry{e2}}
			}
			data, _ := json.Marshal(result)
			msg.Respond(data)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer subStrict.Unsubscribe()

	stdout, _, err := executeCommand("flow", "someproj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(stdout, "---") {
		t.Errorf("output missing separator '---' between flows\n%s", stdout)
	}
	if !strings.Contains(stdout, "fl_M1") || !strings.Contains(stdout, "fl_M2") {
		t.Errorf("output missing one of the flows\n%s", stdout)
	}
}
