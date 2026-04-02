package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/ledger"
	"go.resystems.io/renotify/internal/payload"
	"go.resystems.io/renotify/internal/state"
)

type postArgs struct {
	FlowID   string `json:"flow_id" jsonschema:"active flow ID from register_flow"`
	Title    string `json:"title" jsonschema:"notification title"`
	Body     string `json:"body,omitempty" jsonschema:"notification body text"`
	Priority string `json:"priority,omitempty" jsonschema:"display priority: low, normal (default), or high"`
	Source   string `json:"source,omitempty" jsonschema:"originating pipeline or agent identifier"`
}

type postResult struct {
	NotificationID string `json:"notification_id"`
	Timestamp      string `json:"timestamp"`
}

func (s *Server) registerPostTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "post",
		Description: `Send a fire-and-forget notification to the user's mobile device.
Use this for status updates, alerts, and terminal-return prompts.
The notification is displayed and dismissed — no response is
expected.

Common patterns:
- Alert the user to return to the terminal (e.g. permission
  prompt, passphrase entry, manual verification needed)
- Notify completion of a long-running task
- Report errors or warnings that need human attention

Requires a flow_id from a prior register_flow call.`,
	}, s.handlePost)
}

func (s *Server) handlePost(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args postArgs,
) (*mcp.CallToolResult, *postResult, error) {
	now := time.Now().UTC()
	notificationID := state.GenerateNotificationID()

	// Look up flow context.
	flow, err := s.lookupFlow(args.FlowID)
	if err != nil {
		return nil, nil, err
	}

	priority := payload.PriorityNormal
	if args.Priority != "" {
		priority = payload.Priority(args.Priority)
	}

	req := &payload.NotificationRequest{
		ID:          notificationID,
		FlowID:      args.FlowID,
		DaemonID:    s.daemonID,
		WorkspaceID: flow.WorkspaceID,
		Title:       args.Title,
		Body:        args.Body,
		ResponseTypes: []payload.ResponseType{
			payload.ResponseNone,
		},
		Priority:      priority,
		Source:        args.Source,
		WorkspaceName: flow.DisplayName,
		Timestamp:     now,
	}

	// Publish to JetStream.
	if err := broker.PublishJSON(s.js,
		broker.FlowRequestSubject(s.username, args.FlowID),
		notificationID, req,
	); err != nil {
		return nil, nil, fmt.Errorf("publish notification: %w", err)
	}

	// Insert into ledger.
	if s.db != nil && s.db() != nil {
		s.db().InsertRequest(
			ledger.WriteContext{Username: s.username}, req)
	}

	// Update flow activity.
	if s.db != nil && s.db() != nil {
		s.db().UpdateFlowActivity(args.FlowID, now)
	}

	result := &postResult{
		NotificationID: notificationID,
		Timestamp:      now.Format(time.RFC3339),
	}
	return nil, result, nil
}

// lookupFlow finds an active flow by ID. Returns an error if the
// flow is not found.
func (s *Server) lookupFlow(flowID string) (*ledger.ActiveFlow, error) {
	flows, err := s.db().ListActiveFlows(ledger.ActiveFlowsQuery{
		FlowID: flowID,
	})
	if err != nil {
		return nil, fmt.Errorf("list flows: %w", err)
	}
	for i := range flows {
		if flows[i].FlowID == flowID {
			return &flows[i], nil
		}
	}
	return nil, fmt.Errorf("flow %q not found in active registry", flowID)
}
