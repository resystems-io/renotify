package command

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/payload"
	"go.resystems.io/renotify/cli/internal/testutil"
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

// --- Permission suggestion tests ---

func TestFormatSuggestion_Session(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "addRules",
		"rules": [{"toolName": "Read", "ruleContent": "docs/*"}],
		"behavior": "allow",
		"destination": "session"
	}`)
	got := formatSuggestion(raw)
	want := "Allow Read(docs/*) for session"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSuggestion_LocalSettings(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "addRules",
		"rules": [{"toolName": "Bash", "ruleContent": "npm test"}],
		"behavior": "allow",
		"destination": "localSettings"
	}`)
	got := formatSuggestion(raw)
	want := "Always allow Bash(npm test)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSuggestion_ProjectSettings(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "addRules",
		"rules": [{"toolName": "Edit"}],
		"behavior": "allow",
		"destination": "projectSettings"
	}`)
	got := formatSuggestion(raw)
	want := "Allow Edit in project"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSuggestion_UserSettings(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "addRules",
		"rules": [{"toolName": "Read"}],
		"behavior": "allow",
		"destination": "userSettings"
	}`)
	got := formatSuggestion(raw)
	want := "Always allow Read (global)"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSuggestion_NoRuleContent(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "addRules",
		"rules": [{"toolName": "Bash"}],
		"behavior": "allow",
		"destination": "session"
	}`)
	got := formatSuggestion(raw)
	want := "Allow Bash for session"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFormatSuggestion_NonAddRules_Empty(t *testing.T) {
	raw := json.RawMessage(`{
		"type": "setMode",
		"mode": "acceptEdits",
		"destination": "session"
	}`)
	got := formatSuggestion(raw)
	if got != "" {
		t.Errorf("expected empty for setMode, got %q", got)
	}
}

func TestFormatSuggestion_InvalidJSON_Empty(t *testing.T) {
	got := formatSuggestion(json.RawMessage(`not json`))
	if got != "" {
		t.Errorf("expected empty for invalid JSON, got %q", got)
	}
}

func TestFormatSuggestion_Truncation(t *testing.T) {
	long := strings.Repeat("x", 200)
	raw := json.RawMessage(`{
		"type": "addRules",
		"rules": [{"toolName": "Bash", "ruleContent": "` + long + `"}],
		"behavior": "allow",
		"destination": "session"
	}`)
	got := formatSuggestion(raw)
	if len(got) > 60 {
		t.Errorf("label len %d exceeds 60", len(got))
	}
}

func TestSuggestionLabels_Multiple(t *testing.T) {
	suggestions := []json.RawMessage{
		json.RawMessage(`{
			"type": "addRules",
			"rules": [{"toolName": "Read", "ruleContent": "docs/*"}],
			"behavior": "allow",
			"destination": "session"
		}`),
		json.RawMessage(`{
			"type": "addRules",
			"rules": [{"toolName": "Read"}],
			"behavior": "allow",
			"destination": "localSettings"
		}`),
	}
	labels := suggestionLabels(suggestions)
	if len(labels) != 2 {
		t.Fatalf("got %d labels, want 2", len(labels))
	}
	if labels[0] != "Allow Read(docs/*) for session" {
		t.Errorf("labels[0] = %q", labels[0])
	}
	if labels[1] != "Always allow Read" {
		t.Errorf("labels[1] = %q", labels[1])
	}
}

func TestSuggestionLabels_Empty(t *testing.T) {
	labels := suggestionLabels(nil)
	if labels != nil {
		t.Errorf("expected nil for empty suggestions, got %v",
			labels)
	}
}

func TestSuggestionLabels_SkipsUnsupported(t *testing.T) {
	suggestions := []json.RawMessage{
		json.RawMessage(`{
			"type": "setMode",
			"mode": "acceptEdits",
			"destination": "session"
		}`),
		json.RawMessage(`{
			"type": "addRules",
			"rules": [{"toolName": "Bash"}],
			"behavior": "allow",
			"destination": "session"
		}`),
	}
	labels := suggestionLabels(suggestions)
	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1 (setMode skipped)",
			len(labels))
	}
	if labels[0] != "Allow Bash for session" {
		t.Errorf("labels[0] = %q", labels[0])
	}
}

func TestMatchSuggestion_Found(t *testing.T) {
	suggestions := []json.RawMessage{
		json.RawMessage(`{
			"type": "addRules",
			"rules": [{"toolName": "Read", "ruleContent": "docs/*"}],
			"behavior": "allow",
			"destination": "session"
		}`),
		json.RawMessage(`{
			"type": "addRules",
			"rules": [{"toolName": "Read"}],
			"behavior": "allow",
			"destination": "localSettings"
		}`),
	}
	got := matchSuggestion(suggestions,
		"Allow Read(docs/*) for session")
	if got == nil {
		t.Fatal("expected match, got nil")
	}
	// Verify it returned the first suggestion.
	if !strings.Contains(string(got), "docs/*") {
		t.Errorf("matched wrong suggestion: %s", got)
	}
}

func TestMatchSuggestion_NotFound(t *testing.T) {
	suggestions := []json.RawMessage{
		json.RawMessage(`{
			"type": "addRules",
			"rules": [{"toolName": "Read"}],
			"behavior": "allow",
			"destination": "session"
		}`),
	}
	got := matchSuggestion(suggestions, "nonexistent label")
	if got != nil {
		t.Errorf("expected nil for no match, got %s", got)
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
		// No suggestions → boolean fallback → "Allow"/"Deny".
		if len(req.Actions) != 2 ||
			req.Actions[0] != "Allow" ||
			req.Actions[1] != "Deny" {
			t.Errorf("actions = %v, want [Allow Deny]",
				req.Actions)
		}
		if req.ResponseTypes[0] != payload.ResponseBoolean {
			t.Errorf("response_types = %v, want [boolean]",
				req.ResponseTypes)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request")
	}
}

func TestDispatch_PermissionRequest_WithSuggestions_SendsChoice(
	t *testing.T,
) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

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

			// Respond with the session suggestion choice.
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Action:    "Allow Read(docs/*) for session",
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
		"session_id": "sess_004",
		"cwd": "/home/user/project",
		"hook_event_name": "PermissionRequest",
		"tool_name": "Read",
		"tool_input": {"file_path": "/home/user/project/docs/README.md"},
		"permission_suggestions": [
			{
				"type": "addRules",
				"rules": [{"toolName": "Read", "ruleContent": "docs/*"}],
				"behavior": "allow",
				"destination": "session"
			},
			{
				"type": "addRules",
				"rules": [{"toolName": "Read"}],
				"behavior": "allow",
				"destination": "localSettings"
			}
		]
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

	// Verify the request was sent as choice with correct actions.
	select {
	case req := <-reqCh:
		if req.ResponseTypes[0] != payload.ResponseChoice {
			t.Errorf("response_types = %v, want [choice]",
				req.ResponseTypes)
		}
		// Expected: "Allow once", "Deny" first (visible on
		// notification), then suggestion labels (overflow to
		// in-app "More..." dialog).
		wantActions := []string{
			"Allow once",
			"Deny",
			"Allow Read(docs/*) for session",
			"Always allow Read",
		}
		if len(req.Actions) != len(wantActions) {
			t.Fatalf("actions = %v, want %v",
				req.Actions, wantActions)
		}
		for i, want := range wantActions {
			if req.Actions[i] != want {
				t.Errorf("actions[%d] = %q, want %q",
					i, req.Actions[i], want)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request")
	}

	// Verify the decision includes updatedPermissions.
	var decision hookDecision
	if err := json.Unmarshal(outBuf.Bytes(), &decision); err != nil {
		t.Fatalf("invalid decision JSON: %v\n%s",
			err, outBuf.String())
	}
	if decision.HookSpecificOutput.Decision.Behavior != "allow" {
		t.Errorf("behavior = %q, want allow",
			decision.HookSpecificOutput.Decision.Behavior)
	}
	perms := decision.HookSpecificOutput.Decision.UpdatedPermissions
	if len(perms) != 1 {
		t.Fatalf("updatedPermissions len = %d, want 1",
			len(perms))
	}
	// Verify the echoed suggestion is the session one.
	if !strings.Contains(string(perms[0]), `"session"`) {
		t.Errorf("expected session suggestion, got %s", perms[0])
	}
	if !strings.Contains(string(perms[0]), `docs/*`) {
		t.Errorf("expected docs/* rule, got %s", perms[0])
	}
}

func TestDispatch_PermissionRequest_WithSuggestions_AllowOnce(
	t *testing.T,
) {
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

			// Select "Allow once" (no suggestion).
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Action:    "Allow once",
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
		"tool_name": "Read",
		"tool_input": {"file_path": "/tmp/test"},
		"permission_suggestions": [
			{
				"type": "addRules",
				"rules": [{"toolName": "Read"}],
				"behavior": "allow",
				"destination": "session"
			}
		]
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
	json.Unmarshal(outBuf.Bytes(), &decision)

	if decision.HookSpecificOutput.Decision.Behavior != "allow" {
		t.Errorf("behavior = %q, want allow",
			decision.HookSpecificOutput.Decision.Behavior)
	}
	// "Allow once" should NOT echo updatedPermissions.
	if len(decision.HookSpecificOutput.Decision.UpdatedPermissions) != 0 {
		t.Errorf("Allow once should have no updatedPermissions, got %d",
			len(decision.HookSpecificOutput.Decision.UpdatedPermissions))
	}
}

func TestDispatch_PermissionRequest_WithSuggestions_Deny(
	t *testing.T,
) {
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

			// Select "Deny" from the choice actions.
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Action:    "Deny",
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
		"tool_name": "Bash",
		"tool_input": {"command": "rm -rf /"},
		"permission_suggestions": [
			{
				"type": "addRules",
				"rules": [{"toolName": "Bash"}],
				"behavior": "allow",
				"destination": "session"
			}
		]
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
	json.Unmarshal(outBuf.Bytes(), &decision)

	if decision.HookSpecificOutput.Decision.Behavior != "deny" {
		t.Errorf("behavior = %q, want deny",
			decision.HookSpecificOutput.Decision.Behavior)
	}
}
