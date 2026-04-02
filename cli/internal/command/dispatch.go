package command

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"
	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/payload"
)

// hookInput is the common envelope for all Claude Code hook
// events. Fields are a union of PermissionRequest and
// Notification inputs; unused fields are zero-valued.
type hookInput struct {
	SessionID        string          `json:"session_id"`
	Cwd              string          `json:"cwd"`
	HookEventName    string          `json:"hook_event_name"`
	ToolName         string          `json:"tool_name,omitempty"`
	ToolInput        json.RawMessage `json:"tool_input,omitempty"`
	Title            string          `json:"title,omitempty"`
	Message          string          `json:"message,omitempty"`
	NotificationType string          `json:"notification_type,omitempty"`
}

// hookDecision is the PermissionRequest stdout response.
type hookDecision struct {
	HookSpecificOutput hookSpecificOutput `json:"hookSpecificOutput"`
}

type hookSpecificOutput struct {
	HookEventName string       `json:"hookEventName"`
	Decision      hookBehavior `json:"decision"`
}

type hookBehavior struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message,omitempty"`
}

func newDispatchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dispatch",
		Short: "Claude Code hook dispatcher",
		Long: `Universal hook handler for Claude Code. Reads hook event JSON
from stdin, inspects the hook_event_name field, and dispatches
to the appropriate Renotify flow:

  PermissionRequest → interactive ask (boolean Allow/Deny)
  Notification      → fire-and-forget post
  Other events      → silently ignored (exit 0)

This command has no flags. All context is derived from the
stdin JSON. Configure it in .claude/settings.local.json:

  {
    "hooks": {
      "PermissionRequest": [{
        "hooks": [{
          "type": "command",
          "command": "renotify dispatch",
          "statusMessage": "Awaiting remote approval..."
        }]
      }]
    }
  }

Exit code 1 on error causes Claude Code to fall back to the
normal terminal permission prompt.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"read stdin: %v", err)
			}

			var input hookInput
			if err := json.Unmarshal(data, &input); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"parse hook input: %v", err)
			}

			switch input.HookEventName {
			case "PermissionRequest":
				return dispatchPermissionRequest(
					cmd, app.Config, &input)
			case "Notification":
				return dispatchNotification(
					cmd, app.Config, &input)
			default:
				// Unsupported event — silent no-op.
				return nil
			}
		},
	}

	return cmd
}

// dispatchPermissionRequest handles PermissionRequest hooks by
// sending an interactive ask notification and blocking until the
// mobile user responds with Allow or Deny.
func dispatchPermissionRequest(
	cmd *cobra.Command,
	cfg *config.Config,
	input *hookInput,
) error {
	dir := input.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}

	fc, err := setupFlowFromDir(cfg, dir)
	if err != nil {
		return err
	}
	defer fc.close()

	js, err := natsjs.New(fc.nc)
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"jetstream: %v", err)
	}

	// Create ephemeral response consumer BEFORE publishing to
	// avoid a race where the response arrives before the
	// consumer exists.
	respConsumer, err := js.CreateConsumer(
		cmd.Context(), broker.StreamName,
		natsjs.ConsumerConfig{
			FilterSubject: broker.FlowResponseSubject(
				fc.username, fc.flowID),
			AckPolicy:     natsjs.AckExplicitPolicy,
			DeliverPolicy: natsjs.DeliverNewPolicy,
		})
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"create response consumer: %v", err)
	}

	// Set up signal handling.
	sigCtx, sigStop := signal.NotifyContext(
		cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer sigStop()

	// Start message iterator.
	respMsgs, err := respConsumer.Messages(
		natsjs.PullMaxMessages(1))
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"subscribe response: %v", err)
	}
	defer respMsgs.Stop()

	// Publish via legacy JetStream API (matches post.go/ask.go).
	legacyJS, _ := fc.nc.JetStream()

	now := time.Now().UTC()

	// Publish flow-active lifecycle event.
	activeEvent := &payload.FlowLifecycleEvent{
		FlowID:      fc.flowID,
		DaemonID:    fc.daemonID,
		WorkspaceID: fc.workspaceID,
		Status:      payload.FlowActive,
		Metadata:    fc.workspaceMetadata(),
		Timestamp:   now,
	}
	if err := broker.PublishJSON(legacyJS,
		broker.FlowLifecycleSubject(fc.username, fc.flowID),
		fc.flowID, activeEvent,
	); err != nil {
		return exitcode.Errorf(exitcode.Error,
			"publish lifecycle (active): %v", err)
	}

	// Compose and publish the notification request.
	timeoutDur := cfg.Timeout.DefaultAskTimeout.Duration
	timeoutSec := int(timeoutDur.Seconds())

	req := &payload.NotificationRequest{
		ID:            fc.notificationID,
		FlowID:        fc.flowID,
		DaemonID:      fc.daemonID,
		WorkspaceID:   fc.workspaceID,
		Title:         fmt.Sprintf("Permission: %s", input.ToolName),
		Body:          summariseToolInput(input.ToolName, input.ToolInput),
		ResponseTypes: []payload.ResponseType{payload.ResponseBoolean},
		Priority:      payload.PriorityHigh,
		Source:        hookSource(input.SessionID),
		WorkspaceName: fc.displayName,
		Actions:       []string{"Allow", "Deny"},
		TimeoutSec:    timeoutSec,
		Timestamp:     now,
	}
	if err := broker.PublishJSON(legacyJS,
		broker.FlowRequestSubject(fc.username, fc.flowID),
		fc.notificationID, req,
	); err != nil {
		return exitcode.Errorf(exitcode.Error,
			"publish notification: %v", err)
	}

	// Safety timer: timeout + grace period.
	grace := cfg.Timeout.AskGracePeriod.Duration
	safetyTimer := time.NewTimer(timeoutDur + grace)
	defer safetyTimer.Stop()

	// Channel adapter for the message iterator.
	respCh := make(chan natsjs.Msg, 1)
	go pumpMessages(respMsgs, respCh)

	// publishFailed publishes a failed lifecycle event.
	publishFailed := func() {
		failedEvent := &payload.FlowLifecycleEvent{
			FlowID:      fc.flowID,
			DaemonID:    fc.daemonID,
			WorkspaceID: fc.workspaceID,
			Status:      payload.FlowFailed,
			Timestamp:   time.Now().UTC(),
		}
		broker.PublishJSON(legacyJS,
			broker.FlowLifecycleSubject(fc.username, fc.flowID),
			fc.flowID+"-failed", failedEvent)
	}

	// Wait for response.
	select {
	case msg := <-respCh:
		msg.Ack()
		return handlePermissionResponse(
			cmd, legacyJS, fc, msg.Data())

	case <-safetyTimer.C:
		publishFailed()
		return exitcode.Errorf(exitcode.Error,
			"timeout waiting for permission response")

	case <-sigCtx.Done():
		publishFailed()
		return exitcode.Errorf(exitcode.Error, "interrupted")
	}
}

// handlePermissionResponse processes the mobile response and
// writes the hook decision JSON to stdout.
func handlePermissionResponse(
	cmd *cobra.Command,
	js nats.JetStreamContext,
	fc *flowContext,
	data []byte,
) error {
	// Check for ErrorResponse (timeout, rate limit, etc.).
	var probe struct {
		Code string `json:"code"`
	}
	json.Unmarshal(data, &probe)

	if probe.Code != "" {
		// Daemon-side error — exit 1 for graceful fallback.
		publishCompleted(js, fc, payload.FlowFailed)
		return exitcode.Errorf(exitcode.Error,
			"daemon error: %s", probe.Code)
	}

	// Parse the notification response.
	var resp payload.NotificationResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		publishCompleted(js, fc, payload.FlowFailed)
		return exitcode.Errorf(exitcode.Error,
			"unmarshal response: %v", err)
	}

	// Translate to hook decision.
	decision := hookDecision{
		HookSpecificOutput: hookSpecificOutput{
			HookEventName: "PermissionRequest",
		},
	}

	if resp.Accepted != nil && *resp.Accepted {
		decision.HookSpecificOutput.Decision = hookBehavior{
			Behavior: "allow",
		}
	} else {
		decision.HookSpecificOutput.Decision = hookBehavior{
			Behavior: "deny",
			Message:  "Denied by remote user via Renotify",
		}
	}

	publishCompleted(js, fc, payload.FlowCompleted)

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetEscapeHTML(false)
	return enc.Encode(decision)
}

// dispatchNotification handles Notification hooks by sending a
// fire-and-forget post notification.
func dispatchNotification(
	cmd *cobra.Command,
	cfg *config.Config,
	input *hookInput,
) error {
	dir := input.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	}

	fc, err := setupFlowFromDir(cfg, dir)
	if err != nil {
		return err
	}
	defer fc.close()

	legacyJS, err := fc.nc.JetStream()
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"jetstream: %v", err)
	}

	now := time.Now().UTC()

	// Publish flow-active lifecycle event.
	activeEvent := &payload.FlowLifecycleEvent{
		FlowID:      fc.flowID,
		DaemonID:    fc.daemonID,
		WorkspaceID: fc.workspaceID,
		Status:      payload.FlowActive,
		Metadata:    fc.workspaceMetadata(),
		Timestamp:   now,
	}
	if err := broker.PublishJSON(legacyJS,
		broker.FlowLifecycleSubject(fc.username, fc.flowID),
		fc.flowID, activeEvent,
	); err != nil {
		return exitcode.Errorf(exitcode.Error,
			"publish lifecycle (active): %v", err)
	}

	// Determine priority.
	p := payload.PriorityNormal
	if input.NotificationType == "permission_prompt" {
		p = payload.PriorityHigh
	}

	// Determine title.
	title := input.Title
	if title == "" {
		title = "Notification"
	}

	// Publish notification request.
	req := &payload.NotificationRequest{
		ID:            fc.notificationID,
		FlowID:        fc.flowID,
		DaemonID:      fc.daemonID,
		WorkspaceID:   fc.workspaceID,
		Title:         title,
		Body:          input.Message,
		ResponseTypes: []payload.ResponseType{payload.ResponseNone},
		Priority:      p,
		Source:        hookSource(input.SessionID),
		WorkspaceName: fc.displayName,
		Timestamp:     now,
	}
	if err := broker.PublishJSON(legacyJS,
		broker.FlowRequestSubject(fc.username, fc.flowID),
		fc.notificationID, req,
	); err != nil {
		return exitcode.Errorf(exitcode.Error,
			"publish notification: %v", err)
	}

	// Publish flow-completed lifecycle event.
	publishCompleted(legacyJS, fc, payload.FlowCompleted)

	return nil
}

// publishCompleted publishes a terminal lifecycle event.
func publishCompleted(
	js nats.JetStreamContext,
	fc *flowContext,
	status payload.FlowStatus,
) {
	event := &payload.FlowLifecycleEvent{
		FlowID:      fc.flowID,
		DaemonID:    fc.daemonID,
		WorkspaceID: fc.workspaceID,
		Status:      status,
		Timestamp:   time.Now().UTC(),
	}
	broker.PublishJSON(js,
		broker.FlowLifecycleSubject(fc.username, fc.flowID),
		fc.flowID+"-"+string(status), event)
}

// hookSource returns the source field for hook-dispatched
// notifications. Includes session_id when available.
func hookSource(sessionID string) string {
	if sessionID != "" {
		return "claude-code/" + sessionID
	}
	return "claude-code"
}

// summariseToolInput extracts a human-readable body from the
// tool_input JSON for mobile notification display.
func summariseToolInput(toolName string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return toolName
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return truncate(string(raw), 200)
	}

	getString := func(key string) string {
		v, ok := fields[key]
		if !ok {
			return ""
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
		return ""
	}

	switch toolName {
	case "Bash":
		if cmd := getString("command"); cmd != "" {
			return truncate(cmd, 200)
		}

	case "Edit":
		if fp := getString("file_path"); fp != "" {
			return fp
		}

	case "Write":
		if fp := getString("file_path"); fp != "" {
			return fp
		}

	case "Read":
		if fp := getString("file_path"); fp != "" {
			return fp
		}

	case "Glob":
		pattern := getString("pattern")
		path := getString("path")
		if pattern != "" && path != "" {
			return fmt.Sprintf("%s in %s", pattern, path)
		}
		if pattern != "" {
			return pattern
		}

	case "Grep":
		pattern := getString("pattern")
		path := getString("path")
		if pattern != "" && path != "" {
			return fmt.Sprintf("/%s/ in %s", pattern, path)
		}
		if pattern != "" {
			return fmt.Sprintf("/%s/", pattern)
		}

	case "Agent":
		subType := getString("subagent_type")
		desc := getString("description")
		if subType != "" && desc != "" {
			return fmt.Sprintf("%s: %s", subType, truncate(desc, 180))
		}
		if desc != "" {
			return truncate(desc, 200)
		}

	case "WebFetch":
		if url := getString("url"); url != "" {
			return truncate(url, 200)
		}

	case "WebSearch":
		if query := getString("query"); query != "" {
			return truncate(query, 200)
		}

	case "Skill":
		if skill := getString("skill"); skill != "" {
			if args := getString("args"); args != "" {
				return fmt.Sprintf("%s %s", skill, truncate(args, 180))
			}
			return skill
		}

	case "EnterPlanMode", "ExitPlanMode":
		// These carry allowedPrompts or no useful content.
		return toolName

	case "TaskCreate", "TaskUpdate":
		if desc := getString("description"); desc != "" {
			return truncate(desc, 200)
		}

	case "NotebookEdit":
		if fp := getString("file_path"); fp != "" {
			return fp
		}
	}

	// MCP tools: show tool name prefix.
	if strings.HasPrefix(toolName, "mcp__") {
		compact, _ := json.Marshal(fields)
		return truncate(toolName+" "+string(compact), 200)
	}

	// Fallback: try common descriptive fields before
	// dumping raw JSON.
	for _, key := range []string{
		"description", "prompt", "content", "message",
	} {
		if v := getString(key); v != "" {
			return truncate(v, 200)
		}
	}

	// Last resort: compact JSON.
	compact, err := json.Marshal(fields)
	if err != nil {
		return truncate(string(raw), 200)
	}
	return truncate(string(compact), 200)
}

// truncate returns s truncated to maxLen characters with an
// ellipsis if it exceeds the limit.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
