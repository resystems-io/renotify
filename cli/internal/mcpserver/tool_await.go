package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/internal/payload"
)

type awaitDecisionArgs struct {
	NotificationID string `json:"notification_id" jsonschema:"notification ID returned by the ask tool"`
	TimeoutSec     int    `json:"timeout_sec,omitempty" jsonschema:"max seconds to wait (default: 300)"`
}

func (s *Server) registerAwaitDecisionTool() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "await_decision",
		Description: `Wait for the human's response to a previous ask notification.
Call this immediately after calling ask, passing the
notification_id from the ask result. Blocks until the user
responds on their mobile device or the timeout expires.

Returns the decision:
- "accepted": true/false for boolean responses
- "action": the selected choice label for choice responses
- "text": free-form text for text responses
- If all response fields are absent, the request timed out.

Example flow:
  result = ask(flow_id, title, response_types=["boolean"])
  decision = await_decision(result.notification_id)
  if decision.accepted: proceed
  else: abort`,
	}, s.handleAwaitDecision)
}

func (s *Server) handleAwaitDecision(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args awaitDecisionArgs,
) (*mcp.CallToolResult, *payload.DecisionResource, error) {
	// Check if already decided (response arrived before
	// await_decision was called).
	r := s.decisions.Get(args.NotificationID)
	if r == nil {
		return nil, nil, fmt.Errorf(
			"notification %q not found — call ask first",
			args.NotificationID)
	}
	if r.Decided {
		return nil, r, nil
	}

	// Get the notification channel. The response subscriber
	// (started by ask) closes this channel when the decision
	// is resolved.
	ch := s.decisions.Resolved(args.NotificationID)
	if ch == nil {
		return nil, nil, fmt.Errorf(
			"notification %q not found", args.NotificationID)
	}

	timeoutSec := args.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = int(
			s.cfg.Timeout.DefaultAskTimeout.Duration.Seconds())
	}

	select {
	case <-ch:
		// Decision resolved — read the final state.
		return nil, s.decisions.Get(args.NotificationID), nil
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		// Timeout — return decided with no response fields.
		return nil, &payload.DecisionResource{
			RequestID: args.NotificationID,
			Decided:   true,
			Timestamp: time.Now().UTC(),
		}, nil
	}
}
