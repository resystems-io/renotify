// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package mcpserver

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/payload"
)

// SubscriberMap tracks active response subscriber goroutines,
// keyed by notification ID.
type SubscriberMap struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

// NewSubscriberMap creates an empty SubscriberMap.
func NewSubscriberMap() *SubscriberMap {
	return &SubscriberMap{
		cancels: make(map[string]context.CancelFunc),
	}
}

// Add registers a cancel function for a notification subscriber.
func (sm *SubscriberMap) Add(notificationID string, cancel context.CancelFunc) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.cancels[notificationID] = cancel
}

// Cancel stops and removes a specific subscriber.
func (sm *SubscriberMap) Cancel(notificationID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if cancel, ok := sm.cancels[notificationID]; ok {
		cancel()
		delete(sm.cancels, notificationID)
	}
}

// CancelForFlow cancels all subscribers whose notification IDs
// are in the provided set. Used when a flow is terminated.
func (sm *SubscriberMap) CancelForFlow(notificationIDs []string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for _, id := range notificationIDs {
		if cancel, ok := sm.cancels[id]; ok {
			cancel()
			delete(sm.cancels, id)
		}
	}
}

// CancelAll cancels all active subscribers. Used on Stop.
func (sm *SubscriberMap) CancelAll() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for id, cancel := range sm.cancels {
		cancel()
		delete(sm.cancels, id)
	}
}

// startResponseSubscriber creates an ephemeral JetStream consumer
// on the flow's .response subject and waits for a response. When
// one arrives, it resolves the DecisionResource and emits the MCP
// resource-updated notification.
func (s *Server) startResponseSubscriber(
	flowID, notificationID string,
) {
	ctx, cancel := context.WithCancel(context.Background())
	s.subscribers.Add(notificationID, cancel)

	go func() {
		defer s.subscribers.Cancel(notificationID)

		subject := broker.FlowResponseSubject(s.username, flowID)
		sub, err := s.nc.SubscribeSync(subject)
		if err != nil {
			s.logger.Error("subscribe response",
				"notification_id", notificationID, "err", err)
			return
		}
		defer sub.Unsubscribe()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			msg, err := sub.NextMsg(1 * time.Second)
			if err != nil {
				if err == nats.ErrTimeout {
					continue
				}
				return
			}

			s.handleResponse(flowID, notificationID, msg.Data)
			return
		}
	}()
}

// handleResponse processes a response message for a pending ask.
func (s *Server) handleResponse(
	flowID, notificationID string,
	data []byte,
) {
	// Try to parse as NotificationResponse. If it has a "code"
	// field it's an ErrorResponse (timeout).
	var probe struct {
		Code string `json:"code"`
	}
	json.Unmarshal(data, &probe)

	if probe.Code != "" {
		// ErrorResponse — likely timeout.
		s.decisions.ResolveTimeout(notificationID, time.Now().UTC())
	} else {
		var resp payload.NotificationResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			s.logger.Error("unmarshal response",
				"notification_id", notificationID, "err", err)
			return
		}

		s.decisions.Resolve(notificationID, &resp)

		// Insert into ledger via state service.
		if s.state != nil {
			s.state.InsertResponse(&resp)
		}

		// Note: we do NOT publish FlowCompleted here. A flow
		// can have multiple ask/post cycles. The flow lifecycle
		// is managed by register_flow (active) and
		// terminate_flow (completed).
	}

	// Emit MCP notifications/resources/updated.
	uri := DecisionResourceURI(notificationID)
	s.mcpServer.ResourceUpdated(context.Background(),
		&mcp.ResourceUpdatedNotificationParams{URI: uri})

	s.logger.Info("decision resolved",
		"notification_id", notificationID,
		"flow_id", flowID)
}
