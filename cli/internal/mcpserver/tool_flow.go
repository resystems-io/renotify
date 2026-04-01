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
		Description: `Begin a new Renotify flow for this workspace. Call this FIRST
before using post or ask. Returns a flow_id that you must pass to
all subsequent tool calls. The flow tracks your agent session on
the mobile dashboard and prevents stale-flow reaping.

Call terminate_flow when your work is complete. If you forget,
the flow will be automatically reaped after 5 minutes of
inactivity.

For long-running tasks, call refresh_flow periodically (every
few minutes) to prevent reaping and optionally update the
dashboard label with your current progress.`,
	}, s.handleRegisterFlow)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "refresh_flow",
		Description: `Signal continued activity on a long-running flow. Resets the
stale-flow reaping timer and optionally updates the display
label and metadata on the mobile dashboard. Call this between
major work steps if your task may exceed 5 minutes.

You do NOT need to call this if you are actively calling post
or ask — any tool interaction with the flow resets the timer.`,
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

	flow := &ledger.ActiveFlow{
		FlowID:                flowID,
		Username:              s.username,
		DaemonID:              s.daemonID,
		WorkspaceID:           workspaceID,
		DisplayName:           displayName,
		AbsPath:               args.WorkspacePath,
		Label:                 args.Label,
		Metadata:              meta,
		RegisteredAt:          now,
		LastActivityTimestamp: now,
	}

	if err := s.db().RegisterFlow(flow); err != nil {
		return nil, nil, fmt.Errorf("register flow: %w", err)
	}

	// Publish lifecycle event to NATS.
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

	if err := s.db().RefreshFlow(
		args.FlowID, args.Label, args.Metadata, now,
	); err != nil {
		return nil, nil, fmt.Errorf("refresh flow: %w", err)
	}

	// Publish lifecycle event.
	// Read flow context from ledger for the event fields.
	flows, _ := s.db().ListActiveFlows(ledger.ActiveFlowsQuery{})
	var event payload.FlowLifecycleEvent
	for _, f := range flows {
		if f.FlowID == args.FlowID {
			event = payload.FlowLifecycleEvent{
				FlowID:      args.FlowID,
				DaemonID:    f.DaemonID,
				WorkspaceID: f.WorkspaceID,
				Status:      payload.FlowActive,
				Label:       args.Label,
				Metadata:    args.Metadata,
				Timestamp:   now,
			}
			break
		}
	}
	if event.FlowID != "" {
		broker.PublishJSON(s.js,
			broker.FlowLifecycleSubject(s.username, args.FlowID),
			args.FlowID+"-refresh", &event)
	}

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

	// Read flow context before termination.
	flows, _ := s.db().ListActiveFlows(ledger.ActiveFlowsQuery{})
	var workspaceID string
	for _, f := range flows {
		if f.FlowID == args.FlowID {
			workspaceID = f.WorkspaceID
			break
		}
	}

	if err := s.db().TerminateFlow(
		args.FlowID, args.Status, now,
	); err != nil {
		return nil, nil, fmt.Errorf("terminate flow: %w", err)
	}

	// Publish lifecycle event.
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
