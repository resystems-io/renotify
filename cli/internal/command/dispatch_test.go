package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/payload"
	"go.resystems.io/renotify/internal/testutil"
)

// --- Tool input summarisation tests ---

func TestSummariseToolInput_Bash(t *testing.T) {
	raw := json.RawMessage(`{"command":"npm test"}`)
	got := summariseToolInput("Bash", raw)
	if got != "npm test" {
		t.Errorf("got %q, want %q", got, "npm test")
	}
}

func TestSummariseToolInput_Edit(t *testing.T) {
	raw := json.RawMessage(`{"file_path":"/home/user/main.go"}`)
	got := summariseToolInput("Edit", raw)
	if got != "/home/user/main.go" {
		t.Errorf("got %q, want %q", got, "/home/user/main.go")
	}
}

func TestSummariseToolInput_Write(t *testing.T) {
	raw := json.RawMessage(`{"file_path":"/home/user/new.go"}`)
	got := summariseToolInput("Write", raw)
	if got != "/home/user/new.go" {
		t.Errorf("got %q, want %q", got, "/home/user/new.go")
	}
}

func TestSummariseToolInput_Read(t *testing.T) {
	raw := json.RawMessage(`{"file_path":"/home/user/config.json"}`)
	got := summariseToolInput("Read", raw)
	if got != "/home/user/config.json" {
		t.Errorf("got %q, want %q",
			got, "/home/user/config.json")
	}
}

func TestSummariseToolInput_Glob(t *testing.T) {
	raw := json.RawMessage(
		`{"pattern":"**/*.ts","path":"/home/user/src"}`)
	got := summariseToolInput("Glob", raw)
	want := "**/*.ts in /home/user/src"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSummariseToolInput_Grep(t *testing.T) {
	raw := json.RawMessage(
		`{"pattern":"TODO.*fix","path":"/home/user/src"}`)
	got := summariseToolInput("Grep", raw)
	want := "/TODO.*fix/ in /home/user/src"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSummariseToolInput_Agent(t *testing.T) {
	raw := json.RawMessage(
		`{"subagent_type":"Explore","description":"Find API endpoints"}`)
	got := summariseToolInput("Agent", raw)
	want := "Explore: Find API endpoints"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSummariseToolInput_WebFetch(t *testing.T) {
	raw := json.RawMessage(
		`{"url":"https://docs.example.com/api"}`)
	got := summariseToolInput("WebFetch", raw)
	if got != "https://docs.example.com/api" {
		t.Errorf("got %q", got)
	}
}

func TestSummariseToolInput_WebSearch(t *testing.T) {
	raw := json.RawMessage(`{"query":"go testing patterns"}`)
	got := summariseToolInput("WebSearch", raw)
	if got != "go testing patterns" {
		t.Errorf("got %q", got)
	}
}

func TestSummariseToolInput_MCP(t *testing.T) {
	raw := json.RawMessage(`{"repo":"anthropics/claude"}`)
	got := summariseToolInput("mcp__github__search", raw)
	if !strings.HasPrefix(got, "mcp__github__search") {
		t.Errorf("got %q, want prefix %q",
			got, "mcp__github__search")
	}
}

func TestSummariseToolInput_Unknown(t *testing.T) {
	raw := json.RawMessage(`{"foo":"bar","baz":42}`)
	got := summariseToolInput("SomeNewTool", raw)
	if !strings.Contains(got, "foo") {
		t.Errorf("fallback should contain JSON fields, got %q",
			got)
	}
}

func TestSummariseToolInput_Truncation(t *testing.T) {
	long := strings.Repeat("x", 300)
	raw := json.RawMessage(`{"command":"` + long + `"}`)
	got := summariseToolInput("Bash", raw)
	if len(got) > 200 {
		t.Errorf("got len %d, want <= 200", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("truncated string should end with ...")
	}
}

func TestSummariseToolInput_Empty(t *testing.T) {
	got := summariseToolInput("Bash", nil)
	if got != "Bash" {
		t.Errorf("got %q, want %q", got, "Bash")
	}
}

// --- Hook source tests ---

func TestHookSource_WithSession(t *testing.T) {
	got := hookSource("abc123")
	if got != "claude-code/abc123" {
		t.Errorf("got %q", got)
	}
}

func TestHookSource_NoSession(t *testing.T) {
	got := hookSource("")
	if got != "claude-code" {
		t.Errorf("got %q", got)
	}
}

// --- Command-level tests ---

func TestDispatchHelp(t *testing.T) {
	stdout, _, err := executeCommand("dispatch", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"PermissionRequest", "Notification", "stdin",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %q", want)
		}
	}
}

func TestDispatch_UnsupportedEvent(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"dispatch"})
	root.SetIn(bytes.NewReader(
		[]byte(`{"hook_event_name":"PreToolUse"}`)))

	err := root.Execute()
	if err != nil {
		t.Fatalf("unsupported event should exit 0, got: %v", err)
	}
	if outBuf.Len() != 0 {
		t.Errorf("unsupported event should produce no stdout, got %q",
			outBuf.String())
	}
}

func TestDispatch_InvalidJSON(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")

	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"dispatch"})
	root.SetIn(bytes.NewReader([]byte(`not json`)))

	err := root.Execute()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse hook input") {
		t.Errorf("error = %q, want 'parse hook input'", err)
	}
}

// --- Integration tests ---

func TestDispatch_Notification(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	// Subscribe to capture published messages.
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	var received []*nats.Msg
	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			received = append(received, msg)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	hookJSON := `{
		"session_id": "sess_001",
		"cwd": "/tmp/test",
		"hook_event_name": "Notification",
		"title": "Agent idle",
		"message": "Waiting for input",
		"notification_type": "idle_prompt"
	}`

	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"dispatch"})
	root.SetIn(bytes.NewReader([]byte(hookJSON)))

	if err := root.Execute(); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// No stdout for Notification dispatch.
	if outBuf.Len() != 0 {
		t.Errorf("notification should produce no stdout, got %q",
			outBuf.String())
	}

	// Wait for messages.
	nc.Flush()
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		return len(received) >= 3
	}) {
		t.Fatalf("got %d messages, want >= 3 "+
			"(active lifecycle + request + completed lifecycle)",
			len(received))
	}

	// Verify a .request message was published with correct fields.
	for _, msg := range received {
		if !strings.HasSuffix(msg.Subject, ".request") {
			continue
		}
		var req payload.NotificationRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Fatal(err)
		}
		if req.Title != "Agent idle" {
			t.Errorf("title = %q, want %q",
				req.Title, "Agent idle")
		}
		if req.Body != "Waiting for input" {
			t.Errorf("body = %q, want %q",
				req.Body, "Waiting for input")
		}
		if req.Source != "claude-code/sess_001" {
			t.Errorf("source = %q, want %q",
				req.Source, "claude-code/sess_001")
		}
		if req.Priority != payload.PriorityNormal {
			t.Errorf("priority = %q, want %q",
				req.Priority, payload.PriorityNormal)
		}
		return
	}
	t.Error("no .request message found")
}

func TestDispatch_Notification_PermissionPromptHighPriority(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	var received []*nats.Msg
	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			received = append(received, msg)
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	hookJSON := `{
		"hook_event_name": "Notification",
		"title": "Permission needed",
		"message": "Bash: rm -rf /",
		"notification_type": "permission_prompt"
	}`

	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"dispatch"})
	root.SetIn(bytes.NewReader([]byte(hookJSON)))
	root.Execute()

	nc.Flush()
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		return len(received) >= 3
	}) {
		t.Fatalf("got %d messages, want >= 3", len(received))
	}

	for _, msg := range received {
		if !strings.HasSuffix(msg.Subject, ".request") {
			continue
		}
		var req payload.NotificationRequest
		json.Unmarshal(msg.Data, &req)
		if req.Priority != payload.PriorityHigh {
			t.Errorf("priority = %q, want %q for permission_prompt",
				req.Priority, payload.PriorityHigh)
		}
		return
	}
	t.Error("no .request message found")
}

func TestDispatch_PermissionRequest_Allow(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	// Subscribe to capture the request and send a mock response.
	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			if !strings.HasSuffix(msg.Subject, ".request") {
				return
			}
			// Parse to get flow_id for the response subject.
			var req payload.NotificationRequest
			json.Unmarshal(msg.Data, &req)

			// Respond with "accepted: true".
			accepted := true
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Accepted:  &accepted,
				Timestamp: time.Now().UTC(),
			}
			data, _ := json.Marshal(resp)
			respSubject := strings.Replace(
				msg.Subject, ".request", ".response", 1)

			js, _ := nc.JetStream()
			js.Publish(respSubject, data,
				nats.MsgId(req.ID+"-response"))
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	hookJSON := `{
		"session_id": "sess_002",
		"cwd": "/tmp/test",
		"hook_event_name": "PermissionRequest",
		"tool_name": "Bash",
		"tool_input": {"command": "npm test"}
	}`

	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"dispatch"})
	root.SetIn(bytes.NewReader([]byte(hookJSON)))

	if err := root.Execute(); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	// Verify stdout is allow decision JSON.
	var decision hookDecision
	if err := json.Unmarshal(outBuf.Bytes(), &decision); err != nil {
		t.Fatalf("invalid decision JSON: %v\n%s",
			err, outBuf.String())
	}
	if decision.HookSpecificOutput.HookEventName != "PermissionRequest" {
		t.Errorf("hookEventName = %q",
			decision.HookSpecificOutput.HookEventName)
	}
	if decision.HookSpecificOutput.Decision.Behavior != "allow" {
		t.Errorf("behavior = %q, want %q",
			decision.HookSpecificOutput.Decision.Behavior,
			"allow")
	}
}

func TestDispatch_PermissionRequest_Deny(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			if !strings.HasSuffix(msg.Subject, ".request") {
				return
			}
			var req payload.NotificationRequest
			json.Unmarshal(msg.Data, &req)

			accepted := false
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Accepted:  &accepted,
				Timestamp: time.Now().UTC(),
			}
			data, _ := json.Marshal(resp)
			respSubject := strings.Replace(
				msg.Subject, ".request", ".response", 1)

			js, _ := nc.JetStream()
			js.Publish(respSubject, data,
				nats.MsgId(req.ID+"-response"))
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	hookJSON := `{
		"hook_event_name": "PermissionRequest",
		"tool_name": "Edit",
		"tool_input": {"file_path": "/etc/passwd"}
	}`

	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"dispatch"})
	root.SetIn(bytes.NewReader([]byte(hookJSON)))

	if err := root.Execute(); err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}

	var decision hookDecision
	if err := json.Unmarshal(outBuf.Bytes(), &decision); err != nil {
		t.Fatalf("invalid decision JSON: %v\n%s",
			err, outBuf.String())
	}
	if decision.HookSpecificOutput.Decision.Behavior != "deny" {
		t.Errorf("behavior = %q, want %q",
			decision.HookSpecificOutput.Decision.Behavior,
			"deny")
	}
	if decision.HookSpecificOutput.Decision.Message == "" {
		t.Error("deny decision should include a message")
	}
}

func TestDispatch_PermissionRequest_NotificationFields(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	// Capture the request to verify its fields.
	reqCh := make(chan *payload.NotificationRequest, 1)
	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.flow.>",
		func(msg *nats.Msg) {
			if !strings.HasSuffix(msg.Subject, ".request") {
				return
			}
			var req payload.NotificationRequest
			json.Unmarshal(msg.Data, &req)
			reqCh <- &req

			// Send allow response to unblock dispatch.
			accepted := true
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Accepted:  &accepted,
				Timestamp: time.Now().UTC(),
			}
			data, _ := json.Marshal(resp)
			respSubject := strings.Replace(
				msg.Subject, ".request", ".response", 1)
			js, _ := nc.JetStream()
			js.Publish(respSubject, data,
				nats.MsgId(req.ID+"-response"))
		})
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	hookJSON := `{
		"session_id": "sess_003",
		"cwd": "/home/user/project",
		"hook_event_name": "PermissionRequest",
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf node_modules"}
	}`

	root := NewRoot()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"dispatch"})
	root.SetIn(bytes.NewReader([]byte(hookJSON)))
	root.Execute()

	select {
	case req := <-reqCh:
		if req.Title != "Permission: Bash" {
			t.Errorf("title = %q, want %q",
				req.Title, "Permission: Bash")
		}
		if req.Body != "rm -rf node_modules" {
			t.Errorf("body = %q, want %q",
				req.Body, "rm -rf node_modules")
		}
		if req.Source != "claude-code/sess_003" {
			t.Errorf("source = %q, want %q",
				req.Source, "claude-code/sess_003")
		}
		if req.Priority != payload.PriorityHigh {
			t.Errorf("priority = %q, want %q",
				req.Priority, payload.PriorityHigh)
		}
		if len(req.Actions) != 2 ||
			req.Actions[0] != "Allow" ||
			req.Actions[1] != "Deny" {
			t.Errorf("actions = %v, want [Allow Deny]",
				req.Actions)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request")
	}
}
