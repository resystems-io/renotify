package mcpserver

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/internal/payload"
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
			Description: "Most recent interjection (stop, note, pause) " +
				"for a flow. Use check_interjections or " +
				"await_interjection to consume accumulated " +
				"interjections.",
			MIMEType: "application/json",
		},
		s.handleReadInterjection,
	)
}

// handleReadInterjection serves the latest InterjectionResource.
func (s *Server) handleReadInterjection(
	_ context.Context,
	req *mcp.ReadResourceRequest,
) (*mcp.ReadResourceResult, error) {
	const prefix = "renotify://interjections/"
	if len(req.Params.URI) <= len(prefix) {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	flowID := req.Params.URI[len(prefix):]

	resource := s.interjections.Get(flowID)
	if resource == nil {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}

	data, err := json.Marshal(resource)
	if err != nil {
		return nil, err
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
