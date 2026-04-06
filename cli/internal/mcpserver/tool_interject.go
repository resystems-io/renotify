// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/cli/internal/payload"
)

// --- check_interjections ---

type checkInterjectionsArgs struct {
	FlowID string `json:"flow_id" jsonschema:"active flow ID from register_flow"`
}

type checkInterjectionsResult struct {
	Interjections []payload.InterjectionResource `json:"interjections"`
}

// --- await_interjection ---

type awaitInterjectionArgs struct {
	FlowID     string `json:"flow_id" jsonschema:"active flow ID from register_flow"`
	TimeoutSec int    `json:"timeout_sec,omitempty" jsonschema:"max seconds to wait (default: 300)"`
}

type awaitInterjectionResult struct {
	Interjections []payload.InterjectionResource `json:"interjections"`
}

func (s *Server) registerInterjectionTools() {
	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "check_interjections",
		Description: `Check for interjections (stop, note) sent by the mobile user.
Returns all accumulated interjections since the last check, then
clears the queue. Returns an empty list if none.

Call this between work steps to see if the user has sent any
signals. A "stop" interjection means the user wants you to
abort. A "note" includes context the user wants you to consider.

Non-blocking — returns immediately. For blocking wait, use
await_interjection instead.

Alternative: if you support MCP resource subscriptions, you
can read renotify://interjections/{flow_id} directly instead
of calling this tool. The resource is available immediately
after register_flow (returns [] when empty).

Requires a flow_id from a prior register_flow call.`,
	}, s.handleCheckInterjections)

	mcp.AddTool(s.mcpServer, &mcp.Tool{
		Name: "await_interjection",
		Description: `Wait for the next interjection from the mobile user. Blocks
until an interjection arrives or the timeout expires.

Returns all accumulated interjections on arrival, then clears
the queue. Returns an empty list on timeout.

Use this when you want to be interrupted immediately by user
signals. For non-blocking polling, use check_interjections.

Requires a flow_id from a prior register_flow call.`,
	}, s.handleAwaitInterjection)
}

func (s *Server) handleCheckInterjections(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args checkInterjectionsArgs,
) (*mcp.CallToolResult, *checkInterjectionsResult, error) {
	items := s.interjections.Drain(args.FlowID)
	if items == nil {
		items = []payload.InterjectionResource{}
	}
	return nil, &checkInterjectionsResult{
		Interjections: items,
	}, nil
}

func (s *Server) handleAwaitInterjection(
	_ context.Context,
	_ *mcp.CallToolRequest,
	args awaitInterjectionArgs,
) (*mcp.CallToolResult, *awaitInterjectionResult, error) {
	// Check if there are already accumulated interjections.
	items := s.interjections.Drain(args.FlowID)
	if len(items) > 0 {
		return nil, &awaitInterjectionResult{
			Interjections: items,
		}, nil
	}

	ch := s.interjections.Notified(args.FlowID)
	if ch == nil {
		return nil, nil, fmt.Errorf(
			"flow %q not found — call register_flow first",
			args.FlowID)
	}

	timeoutSec := args.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = int(
			s.cfg.Timeout.DefaultAskTimeout.Duration.Seconds())
	}

	select {
	case <-ch:
		items = s.interjections.Drain(args.FlowID)
		if items == nil {
			items = []payload.InterjectionResource{}
		}
		return nil, &awaitInterjectionResult{
			Interjections: items,
		}, nil
	case <-time.After(time.Duration(timeoutSec) * time.Second):
		return nil, &awaitInterjectionResult{
			Interjections: []payload.InterjectionResource{},
		}, nil
	}
}
