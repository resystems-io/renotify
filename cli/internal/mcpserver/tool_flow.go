package mcpserver

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/ledger"
	"go.resystems.io/renotify/internal/payload"
	"go.resystems.io/renotify/internal/state"
)

// --- register_flow ---

type registerFlowArgs struct {
	WorkspacePath string            `json:"workspace_path" jsonschema:"absolute filesystem path of the workspace directory"`
	Label         string            `json:"label,omitempty" jsonschema:"human-readable flow name for the mobile dashboard"`
	Metadata      map[string]string `json:"metadata,omitempty" jsonschema:"arbitrary key-value context (e.g. branch or commit)"`
}

type registerFlowResult struct {
	FlowID      string `json:"flow_id"`
	WorkspaceID string `json:"workspace_id"`
	Timestamp   string `json:"timestamp"`
}

// --- refresh_flow ---

type refreshFlowArgs struct {
	FlowID   string            `json:"flow_id" jsonschema:"active flow ID from register_flow"`
	Label    string            `json:"label,omitempty" jsonschema:"updated display name"`
	Metadata map[string]string `json:"metadata,omitempty" jsonschema:"updated key-value context (merged with existing)"`
}

type refreshFlowResult struct {
	FlowID    string `json:"flow_id"`
	Timestamp string `json:"timestamp"`
}

// --- terminate_flow ---

type terminateFlowArgs struct {
	FlowID string `json:"flow_id" jsonschema:"active flow ID to terminate"`
	Status string `json:"status" jsonschema:"terminal status: completed or failed"`
}

type terminateFlowResult struct {
	FlowID    string `json:"flow_id"`
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

func (s *Server) registerFlowTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "register_flow",
		Description: `Begin a new Renotify flow. Call this FIRST — before post, ask,
or any other tool. Returns a flow_id you must pass to all
subsequent calls.

workspace_path must be an absolute filesystem path (e.g.
"/home/user/project", not "." or "project").

Always call terminate_flow when done. Flows are automatically
reaped after 5 minutes of inactivity if you forget.

For long-running tasks (> 5 min between tool calls), call
refresh_flow periodically to prevent reaping.

Interjection resource: after registration, the resource at
renotify://interjections/{flow_id} is immediately available.
Read it between work steps to check for user signals (stop,
note). If you support resource subscriptions, subscribe to
it for push notifications when the user sends a signal.`,
	}, s.handleRegisterFlow)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "refresh_flow",
		Description: `Signal continued activity on a long-running flow. Resets the
stale-flow reaping timer and optionally updates the display
label and metadata on the mobile dashboard. Call this between
major work steps if your task may exceed 5 minutes.

You do NOT need to call this if you are actively calling post
or ask — any tool interaction with the flow resets the timer.

For long-running tasks, you should call this tool periodically
and pass in metadata (key, value) pair that are informative to
the user and describe how the task is progressing.

Returns an error if the flow has been reaped (expired due to
inactivity). If this happens, call register_flow to start a
new flow.`,
	}, s.handleRefreshFlow)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "terminate_flow",
		Description: `End a flow. Always call this when your work is complete
(status: "completed") or when you encounter an unrecoverable
error (status: "failed"). This removes the flow from the mobile
dashboard and cleans up resources.`,
	}, s.handleTerminateFlow)
}

func (s *Server) handleRegisterFlow(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args registerFlowArgs,
) (*mcp.CallToolResult, *registerFlowResult, error) {
	now := time.Now().UTC()
	flowID := state.GenerateFlowID()
	workspaceID := state.WorkspaceID(s.daemonID, args.WorkspacePath)
	displayName := path.Base(args.WorkspacePath)

	// Add workspace metadata for the registry (C-10).
	meta := args.Metadata
	if meta == nil {
		meta = make(map[string]string)
	}
	meta[payload.MetaDisplayName] = displayName
	meta[payload.MetaAbsPath] = args.WorkspacePath

	// Register interjection queue for this flow (C-11).
	s.interjections.Register(flowID)

	// Publish lifecycle event to NATS. The registry's lifecycle
	// consumer handles the DB write (C-10). MCP tools do not
	// write to active_flows directly to avoid races.
	event := &payload.FlowLifecycleEvent{
		FlowID:      flowID,
		DaemonID:    s.daemonID,
		WorkspaceID: workspaceID,
		Status:      payload.FlowActive,
		Label:       args.Label,
		Metadata:    meta,
		Timestamp:   now,
	}
	broker.PublishJSON(s.js,
		broker.FlowLifecycleSubject(s.username, flowID),
		flowID, event)

	result := &registerFlowResult{
		FlowID:      flowID,
		WorkspaceID: workspaceID,
		Timestamp:   now.Format(time.RFC3339),
	}
	return nil, result, nil
}

func (s *Server) handleRefreshFlow(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args refreshFlowArgs,
) (*mcp.CallToolResult, *refreshFlowResult, error) {
	now := time.Now().UTC()

	// Publish lifecycle event to NATS. The registry's lifecycle
	// consumer handles the DB write (C-10).
	// Read flow context from ledger for the event fields.
	flows, _ := s.db().ListActiveFlows(ledger.ActiveFlowsQuery{
		FlowID: args.FlowID,
	})
	var event payload.FlowLifecycleEvent
	for _, f := range flows {
		if f.FlowID == args.FlowID {
			// Merge new metadata with existing. New keys
			// overwrite, existing keys are preserved.
			merged := make(map[string]string)
			for k, v := range f.Metadata {
				merged[k] = v
			}
			for k, v := range args.Metadata {
				merged[k] = v
			}

			// Preserve existing label when not provided.
			label := args.Label
			if label == "" {
				label = f.Label
			}

			event = payload.FlowLifecycleEvent{
				FlowID:      args.FlowID,
				DaemonID:    f.DaemonID,
				WorkspaceID: f.WorkspaceID,
				Status:      payload.FlowActive,
				Label:       label,
				Metadata:    merged,
				Timestamp:   now,
			}
			break
		}
	}
	if event.FlowID == "" {
		return nil, nil, fmt.Errorf(
			"flow %q not found (may have been reaped after "+
				"inactivity) — call register_flow to start a new flow",
			args.FlowID)
	}

	msgID := fmt.Sprintf("%s-refresh-%d",
		args.FlowID, now.UnixMilli())
	broker.PublishJSON(s.js,
		broker.FlowLifecycleSubject(s.username, args.FlowID),
		msgID, &event)

	result := &refreshFlowResult{
		FlowID:    args.FlowID,
		Timestamp: now.Format(time.RFC3339),
	}
	return nil, result, nil
}

func (s *Server) handleTerminateFlow(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args terminateFlowArgs,
) (*mcp.CallToolResult, *terminateFlowResult, error) {
	if args.Status != "completed" && args.Status != "failed" {
		return nil, nil, fmt.Errorf(
			"status must be 'completed' or 'failed', got %q",
			args.Status)
	}

	now := time.Now().UTC()

	// Look up flow context for the lifecycle event.
	flows, _ := s.db().ListActiveFlows(ledger.ActiveFlowsQuery{})
	var workspaceID string
	for _, f := range flows {
		if f.FlowID == args.FlowID {
			workspaceID = f.WorkspaceID
			break
		}
	}

	// Clean up interjection queue (C-11).
	s.interjections.Remove(args.FlowID)

	// Publish lifecycle event to NATS. The registry's lifecycle
	// consumer handles the DB write (C-10).
	event := &payload.FlowLifecycleEvent{
		FlowID:      args.FlowID,
		DaemonID:    s.daemonID,
		WorkspaceID: workspaceID,
		Status:      payload.FlowStatus(args.Status),
		Timestamp:   now,
	}
	broker.PublishJSON(s.js,
		broker.FlowLifecycleSubject(s.username, args.FlowID),
		args.FlowID+"-"+args.Status, event)

	result := &terminateFlowResult{
		FlowID:    args.FlowID,
		Status:    args.Status,
		Timestamp: now.Format(time.RFC3339),
	}
	return nil, result, nil
}
