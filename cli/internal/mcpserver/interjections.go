// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package mcpserver

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/cli/internal/payload"
)

// flowInterjections holds the accumulated interjections and
// notification channel for a single flow.
type flowInterjections struct {
	queue  []payload.InterjectionResource
	notify chan struct{}
}

// InterjectionStore is a thread-safe in-memory store that
// accumulates InterjectionResource entries per flow. Unlike
// DecisionStore (one-shot per notification), interjections
// accumulate — a user may send multiple notes or a stop.
type InterjectionStore struct {
	mu    sync.Mutex
	flows map[string]*flowInterjections
}

// NewInterjectionStore creates an empty InterjectionStore.
func NewInterjectionStore() *InterjectionStore {
	return &InterjectionStore{
		flows: make(map[string]*flowInterjections),
	}
}

// Register creates an empty interjection queue for a flow.
// Called when register_flow is invoked.
func (is *InterjectionStore) Register(flowID string) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.flows[flowID] = &flowInterjections{
		notify: make(chan struct{}),
	}
}

// Registered reports whether a flow has been registered in
// the interjection store (even if no interjections have
// arrived yet).
func (is *InterjectionStore) Registered(flowID string) bool {
	is.mu.Lock()
	defer is.mu.Unlock()
	_, ok := is.flows[flowID]
	return ok
}

// Append adds an interjection to the flow's queue and signals
// any waiting await_interjection caller. A new notification
// channel is created for the next wait cycle.
func (is *InterjectionStore) Append(
	flowID string,
	r payload.InterjectionResource,
) {
	is.mu.Lock()
	defer is.mu.Unlock()
	fi, ok := is.flows[flowID]
	if !ok {
		return
	}
	fi.queue = append(fi.queue, r)
	close(fi.notify)
	fi.notify = make(chan struct{})
}

// Drain returns and removes all accumulated interjections for
// a flow. Returns nil if the flow is not registered or the
// queue is empty.
func (is *InterjectionStore) Drain(
	flowID string,
) []payload.InterjectionResource {
	is.mu.Lock()
	defer is.mu.Unlock()
	fi, ok := is.flows[flowID]
	if !ok || len(fi.queue) == 0 {
		return nil
	}
	result := fi.queue
	fi.queue = nil
	return result
}

// Get returns the most recent interjection without draining.
// Used by the resource template handler. Returns nil if the
// flow has no interjections.
func (is *InterjectionStore) Get(
	flowID string,
) *payload.InterjectionResource {
	is.mu.Lock()
	defer is.mu.Unlock()
	fi, ok := is.flows[flowID]
	if !ok || len(fi.queue) == 0 {
		return nil
	}
	r := fi.queue[len(fi.queue)-1]
	return &r
}

// Notified returns a channel that is closed when a new
// interjection arrives for the flow. Returns nil if the flow
// is not registered.
func (is *InterjectionStore) Notified(
	flowID string,
) <-chan struct{} {
	is.mu.Lock()
	defer is.mu.Unlock()
	fi, ok := is.flows[flowID]
	if !ok {
		return nil
	}
	return fi.notify
}

// Remove deletes a flow's interjection queue and channel.
// Called when terminate_flow is invoked.
func (is *InterjectionStore) Remove(flowID string) {
	is.mu.Lock()
	defer is.mu.Unlock()
	delete(is.flows, flowID)
}

// InterjectionResourceURI returns the MCP resource URI for a
// flow's interjections.
func InterjectionResourceURI(flowID string) string {
	return "renotify://interjections/" + flowID
}

// registerInterjectionTemplate registers the InterjectionResource
// template on the MCP server.
func (s *Server) registerInterjectionTemplate() {
	s.mcpServer.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "renotify://interjections/{flow_id}",
			Name:        "interjection",
			Title:       "Interjection Resource",
			Description: "Most recent interjection (stop, note) for a " +
				"flow. Available immediately after register_flow " +
				"(returns [] when empty). Subscribe for push " +
				"notifications, or poll between work steps. " +
				"A stop interjection means abort; a note includes " +
				"context to consider.",
			MIMEType: "application/json",
		},
		s.handleReadInterjection,
	)
}

// handleReadInterjection serves the latest InterjectionResource.
// Returns an empty array when the flow is registered but no
// interjections have arrived yet — this allows MCP clients to
// subscribe to the resource immediately after register_flow.
func (s *Server) handleReadInterjection(
	_ context.Context,
	req *mcp.ReadResourceRequest,
) (*mcp.ReadResourceResult, error) {
	const prefix = "renotify://interjections/"
	if len(req.Params.URI) <= len(prefix) {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	flowID := req.Params.URI[len(prefix):]

	if !s.interjections.Registered(flowID) {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}

	resource := s.interjections.Get(flowID)

	var data []byte
	var err error
	if resource == nil {
		data = []byte("[]")
	} else {
		data, err = json.Marshal(resource)
		if err != nil {
			return nil, err
		}
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(data),
			},
		},
	}, nil
}
