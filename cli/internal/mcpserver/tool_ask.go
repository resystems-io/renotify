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
Returns immediately with a notification_id and a resource_uri.
The human's decision arrives asynchronously — subscribe to
notifications/resources/updated and read the DecisionResource at
the returned URI to obtain the result.

Common patterns:
- Remote approval: response_types ["boolean"] for approve/reject
- Remote choice: response_types ["choice"] with actions list
- Remote input: response_types ["text"] for free-form feedback
- Multi-modal: combine types (e.g. ["boolean", "text"])

Reading the decision:
1. Call ask — receive resource_uri
2. Wait for notifications/resources/updated event for that URI
3. Read the resource — decided:true means the user responded
4. If decided:true with no response fields, the request timed out

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

	// Look up flow context.
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

	// Insert into ledger.
	if s.db != nil && s.db() != nil {
		s.db().InsertRequest(
			ledger.WriteContext{Username: s.username}, req)
	}

	// Create pending DecisionResource.
	s.decisions.Register(notificationID, now)

	// Start response subscriber goroutine.
	s.startResponseSubscriber(args.FlowID, notificationID)

	// Update flow activity.
	if s.db != nil && s.db() != nil {
		s.db().UpdateFlowActivity(args.FlowID, now)
	}

	resourceURI := DecisionResourceURI(notificationID)
	result := &askResult{
		NotificationID: notificationID,
		ResourceURI:    resourceURI,
		Timestamp:      now.Format(time.RFC3339),
	}
	return nil, result, nil
}
