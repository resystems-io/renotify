package mcpserver

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"go.resystems.io/renotify/internal/payload"
)

// DecisionStore is a thread-safe in-memory store for pending
// DecisionResource instances. Each ask tool call creates a
// pending resource; it is resolved when the mobile user responds.
type DecisionStore struct {
	mu        sync.RWMutex
	resources map[string]*payload.DecisionResource
	resolved  map[string]chan struct{}
}

// NewDecisionStore creates an empty DecisionStore.
func NewDecisionStore() *DecisionStore {
	return &DecisionStore{
		resources: make(map[string]*payload.DecisionResource),
		resolved:  make(map[string]chan struct{}),
	}
}

// Register creates a pending (undecided) DecisionResource with
// a notification channel that is closed when the decision is
// resolved.
func (ds *DecisionStore) Register(notificationID string, ts time.Time) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	ds.resources[notificationID] = &payload.DecisionResource{
		RequestID: notificationID,
		Decided:   false,
		Timestamp: ts,
	}
	ds.resolved[notificationID] = make(chan struct{})
}

// Resolve marks a DecisionResource as decided and copies the
// response fields. Returns false if the resource was not found.
func (ds *DecisionStore) Resolve(
	notificationID string,
	resp *payload.NotificationResponse,
) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	r, ok := ds.resources[notificationID]
	if !ok {
		return false
	}
	r.Decided = true
	r.Accepted = resp.Accepted
	r.Action = resp.Action
	r.Text = resp.Text
	r.Timestamp = resp.Timestamp
	if ch, ok := ds.resolved[notificationID]; ok {
		close(ch)
	}
	return true
}

// ResolveTimeout marks a DecisionResource as decided with no
// response fields (indicating timeout). Returns false if not found.
func (ds *DecisionStore) ResolveTimeout(
	notificationID string, ts time.Time,
) bool {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	r, ok := ds.resources[notificationID]
	if !ok {
		return false
	}
	r.Decided = true
	r.Timestamp = ts
	if ch, ok := ds.resolved[notificationID]; ok {
		close(ch)
	}
	return true
}

// Get returns a copy of the DecisionResource, or nil if not found.
func (ds *DecisionStore) Get(notificationID string) *payload.DecisionResource {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	r, ok := ds.resources[notificationID]
	if !ok {
		return nil
	}
	// Return a copy to avoid race conditions.
	copy := *r
	return &copy
}

// Resolved returns a channel that is closed when the decision
// for the given notification ID is resolved. Returns nil if the
// notification is not found.
func (ds *DecisionStore) Resolved(notificationID string) <-chan struct{} {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.resolved[notificationID]
}

// Remove deletes a DecisionResource from the store.
func (ds *DecisionStore) Remove(notificationID string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()
	delete(ds.resources, notificationID)
	delete(ds.resolved, notificationID)
}

// DecisionResourceURI returns the MCP resource URI for a
// notification's decision.
func DecisionResourceURI(notificationID string) string {
	return "renotify://decisions/" + notificationID
}

// registerDecisionTemplate registers the DecisionResource
// template on the MCP server.
func (s *Server) registerDecisionTemplate() {
	s.mcpServer.AddResourceTemplate(
		&mcp.ResourceTemplate{
			URITemplate: "renotify://decisions/{notification_id}",
			Name:        "decision",
			Title:       "Decision Resource",
			Description: "Human decision for a pending ask notification. " +
				"Read this resource after receiving a " +
				"notifications/resources/updated event to obtain " +
				"the user's response.",
			MIMEType: "application/json",
		},
		s.handleReadDecision,
	)
}

// handleReadDecision serves a DecisionResource for the given URI.
func (s *Server) handleReadDecision(
	_ context.Context,
	req *mcp.ReadResourceRequest,
) (*mcp.ReadResourceResult, error) {
	// Extract notification_id from URI:
	// renotify://decisions/{notification_id}
	const prefix = "renotify://decisions/"
	if len(req.Params.URI) <= len(prefix) {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	notificationID := req.Params.URI[len(prefix):]

	resource := s.decisions.Get(notificationID)
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
