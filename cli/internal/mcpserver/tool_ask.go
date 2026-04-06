package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/payload"
	"go.resystems.io/renotify/cli/internal/state"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

type askArgs struct {
	FlowID        string   `json:"flow_id" jsonschema:"active flow ID from register_flow"`
	Title         string   `json:"title" jsonschema:"notification title"`
	Body          string   `json:"body,omitempty" jsonschema:"notification body text"`
	ResponseTypes []string `json:"response_types" jsonschema:"accepted response types: boolean, choice, text"`
	Priority      string   `json:"priority,omitempty" jsonschema:"display priority: low, normal (default), or high"`
	Source        string   `json:"source,omitempty" jsonschema:"originating pipeline or agent identifier"`
	Actions       []string `json:"actions,omitempty" jsonschema:"choice labels (required when response_types includes choice)"`
	TimeoutSec    int      `json:"timeout_sec,omitempty" jsonschema:"server-side timeout in seconds (default: 300)"`
}

type askResult struct {
	NotificationID string `json:"notification_id"`
	ResourceURI    string `json:"resource_uri"`
	Timestamp      string `json:"timestamp"`
}

func (s *Server) registerAskTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "ask",
		Description: `Send an interactive notification that requires a human response.
Returns immediately with a notification_id. To wait for the
user's response, call await_decision with the notification_id.

Usage:
  result = ask(flow_id=..., title=..., response_types=["boolean"])
  decision = await_decision(notification_id=result.notification_id)

Common patterns:
- Remote approval: response_types ["boolean"] for approve/reject
- Remote choice: response_types ["choice"] with actions ["A","B"]
- Remote input: response_types ["text"] for free-form feedback
- Multi-modal: combine types e.g. ["boolean", "text"]

Alternatively, MCP clients that support resource subscriptions
can subscribe to the returned resource_uri and receive a
notifications/resources/updated event when the decision arrives,
then read the DecisionResource directly.

Requires a flow_id from a prior register_flow call.`,
	}, s.handleAsk)
}

func (s *Server) handleAsk(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args askArgs,
) (*mcp.CallToolResult, *askResult, error) {
	now := time.Now().UTC()
	notificationID := state.GenerateNotificationID()

	// Look up flow context via state service.
	flow, err := s.lookupFlow(args.FlowID)
	if err != nil {
		return nil, nil, err
	}

	// Convert response types.
	responseTypes := make([]payload.ResponseType, len(args.ResponseTypes))
	for i, rt := range args.ResponseTypes {
		responseTypes[i] = payload.ResponseType(rt)
	}

	priority := payload.PriorityNormal
	if args.Priority != "" {
		priority = payload.Priority(args.Priority)
	}

	timeoutSec := args.TimeoutSec
	if timeoutSec == 0 {
		timeoutSec = int(s.cfg.Timeout.DefaultAskTimeout.Duration.Seconds())
	}

	req := &payload.NotificationRequest{
		ID:            notificationID,
		FlowID:        args.FlowID,
		DaemonID:      s.daemonID,
		WorkspaceID:   flow.WorkspaceID,
		Title:         args.Title,
		Body:          args.Body,
		ResponseTypes: responseTypes,
		Priority:      priority,
		Source:        args.Source,
		WorkspaceName: flow.DisplayName,
		Actions:       args.Actions,
		TimeoutSec:    timeoutSec,
		Timestamp:     now,
	}

	// Publish to JetStream.
	if err := broker.PublishJSON(s.js,
		broker.FlowRequestSubject(s.username, args.FlowID),
		notificationID, req,
	); err != nil {
		return nil, nil, fmt.Errorf("publish notification: %w", err)
	}

	// Insert into ledger via state service.
	if s.state != nil {
		s.state.InsertRequest(statesvc.InsertRequestCmd{
			Username:      s.username,
			FlowLabel:     flow.Label,
			WorkspaceName: flow.DisplayName,
			WorkspacePath: flow.AbsPath,
			Request:       *req,
		})
	}

	// Create pending DecisionResource.
	s.decisions.Register(notificationID, now)

	// Start response subscriber goroutine.
	s.startResponseSubscriber(args.FlowID, notificationID)

	// Start daemon-side timeout timer (D-27, C-11).
	s.startTimeoutTimer(args.FlowID, notificationID, timeoutSec)

	// Update flow activity via state service.
	if s.state != nil {
		s.state.UpdateActivity(args.FlowID, now)
	}

	resourceURI := DecisionResourceURI(notificationID)
	result := &askResult{
		NotificationID: notificationID,
		ResourceURI:    resourceURI,
		Timestamp:      now.Format(time.RFC3339),
	}
	return nil, result, nil
}
