// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	natsjs "github.com/nats-io/nats.go/jetstream"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/ledger"
	"go.resystems.io/renotify/cli/internal/payload"
)

// startLifecycleConsumer binds to the daemon-lifecycle JetStream
// consumer and starts a goroutine that processes lifecycle events.
func (s *Service) startLifecycleConsumer(ctx context.Context) error {
	js, err := natsjs.New(s.nc)
	if err != nil {
		return fmt.Errorf("registry: create jetstream: %w", err)
	}

	consumerName := broker.LifecycleConsumerName(s.username)
	consumer, err := js.Consumer(ctx, broker.StreamName, consumerName)
	if err != nil {
		return fmt.Errorf("registry: bind consumer %s: %w",
			consumerName, err)
	}

	iter, err := consumer.Messages()
	if err != nil {
		return fmt.Errorf("registry: start messages: %w", err)
	}

	s.logger.Info("lifecycle consumer started",
		"consumer", consumerName)

	go s.consumeLifecycle(ctx, iter)
	return nil
}

// consumeLifecycle reads messages from the JetStream iterator
// until the context is cancelled. The consumer receives lifecycle,
// request, and response messages and routes each to the
// appropriate handler based on the subject suffix.
func (s *Service) consumeLifecycle(
	ctx context.Context,
	iter natsjs.MessagesContext,
) {
	defer iter.Stop()

	for {
		msg, err := iter.Next()
		if err != nil {
			// Context cancelled or iterator drained.
			select {
			case <-ctx.Done():
				return
			default:
				s.logger.Error("lifecycle next", "err", err)
				return
			}
		}

		s.processMessage(msg)
	}
}

// subjectSuffix extracts the last segment of a NATS subject.
// e.g. "resystems.renotify.alice.flow.fl_X.lifecycle" → "lifecycle"
func subjectSuffix(subject string) string {
	if i := strings.LastIndexByte(subject, '.'); i >= 0 {
		return subject[i+1:]
	}
	return subject
}

// processMessage routes a JetStream message to the appropriate
// handler based on the subject suffix (lifecycle, request, or
// response).
func (s *Service) processMessage(msg natsjs.Msg) {
	switch subjectSuffix(msg.Subject()) {
	case "lifecycle":
		s.processLifecycleEvent(msg)
	case "request":
		s.processRequest(msg)
	case "response":
		s.processResponse(msg)
	default:
		s.logger.Warn("unknown subject suffix",
			"subject", msg.Subject())
		msg.Ack()
	}
}

// processLifecycleEvent handles a single lifecycle message.
func (s *Service) processLifecycleEvent(msg natsjs.Msg) {
	var event payload.FlowLifecycleEvent
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		s.logger.Error("lifecycle unmarshal",
			"err", err, "subject", msg.Subject())
		msg.Ack()
		return
	}

	switch event.Status {
	case payload.FlowActive:
		s.handleActive(&event)
	case payload.FlowCompleted, payload.FlowFailed:
		s.handleTerminal(&event)
	default:
		s.logger.Warn("lifecycle unknown status",
			"status", event.Status,
			"flow_id", event.FlowID)
	}

	msg.Ack()
	s.rebuildWorkspaceSnapshot()
}

// processRequest records a NotificationRequest in the history
// ledger. This captures notifications from all sources (CLI,
// MCP, dispatch) via the JetStream stream. INSERT OR IGNORE
// ensures idempotency with MCP's direct svc.insert-request calls.
func (s *Service) processRequest(msg natsjs.Msg) {
	var req payload.NotificationRequest
	if err := json.Unmarshal(msg.Data(), &req); err != nil {
		s.logger.Error("request unmarshal",
			"err", err, "subject", msg.Subject())
		msg.Ack()
		return
	}

	// Build WriteContext from the active flow (if registered).
	wc := ledger.WriteContext{Username: s.username}
	flow, err := s.dbFunc().LookupActiveFlow(req.FlowID)
	if err == nil && flow != nil {
		wc.FlowLabel = flow.Label
		wc.WorkspaceName = flow.DisplayName
		wc.WorkspacePath = flow.AbsPath
	}

	if err := s.dbFunc().InsertRequest(wc, &req); err != nil {
		s.logger.Error("insert request",
			"id", req.ID, "err", err)
	}

	msg.Ack()
}

// processResponse records a NotificationResponse in the history
// ledger. ErrorResponses (timeouts) are ignored — only human
// responses are persisted.
func (s *Service) processResponse(msg natsjs.Msg) {
	// Discriminate ErrorResponse from NotificationResponse.
	if isErrorResponse(msg.Data()) {
		msg.Ack()
		return
	}

	var resp payload.NotificationResponse
	if err := json.Unmarshal(msg.Data(), &resp); err != nil {
		s.logger.Error("response unmarshal",
			"err", err, "subject", msg.Subject())
		msg.Ack()
		return
	}

	if err := s.dbFunc().InsertResponse(&resp); err != nil {
		s.logger.Error("insert response",
			"request_id", resp.RequestID, "err", err)
	}

	msg.Ack()
}

// isErrorResponse checks whether raw JSON contains an
// ErrorResponse (has a non-empty "code" field) rather than a
// NotificationResponse.
func isErrorResponse(data []byte) bool {
	var probe struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false
	}
	return probe.Code != ""
}

// handleActive registers a new flow or refreshes an existing one.
func (s *Service) handleActive(event *payload.FlowLifecycleEvent) {
	displayName := event.Metadata[payload.MetaDisplayName]
	absPath := event.Metadata[payload.MetaAbsPath]

	flow := &ledger.ActiveFlow{
		FlowID:                event.FlowID,
		Username:              s.username,
		DaemonID:              event.DaemonID,
		WorkspaceID:           event.WorkspaceID,
		DisplayName:           displayName,
		AbsPath:               absPath,
		Label:                 event.Label,
		Metadata:              event.Metadata,
		RegisteredAt:          event.Timestamp,
		LastActivityTimestamp: event.Timestamp,
	}

	err := s.dbFunc().RegisterFlow(flow)
	if err != nil {
		// If the flow already exists (INSERT conflict), treat
		// as a refresh instead.
		s.dbFunc().RefreshFlow(
			event.FlowID,
			event.Label,
			event.Metadata,
			event.Timestamp,
		)
	}

	s.logger.Debug("lifecycle active",
		"flow_id", event.FlowID,
		"workspace_id", event.WorkspaceID)
}

// handleTerminal removes a flow from the active registry.
func (s *Service) handleTerminal(event *payload.FlowLifecycleEvent) {
	if err := s.dbFunc().TerminateFlow(
		event.FlowID,
		string(event.Status),
		event.Timestamp,
	); err != nil {
		s.logger.Error("lifecycle terminate",
			"flow_id", event.FlowID, "err", err)
		return
	}

	s.logger.Debug("lifecycle terminal",
		"flow_id", event.FlowID,
		"status", event.Status)
}

// publishFailedLifecycle publishes a FlowLifecycleEvent with
// status "failed" for a reaped flow so the mobile app and any
// listening CLI processes are notified.
func (s *Service) publishFailedLifecycle(
	f ledger.ActiveFlow,
	ts time.Time,
) {
	event := payload.FlowLifecycleEvent{
		FlowID:      f.FlowID,
		DaemonID:    f.DaemonID,
		WorkspaceID: f.WorkspaceID,
		Status:      payload.FlowFailed,
		Timestamp:   ts,
	}

	data, err := json.Marshal(event)
	if err != nil {
		s.logger.Error("marshal reaped lifecycle", "err", err)
		return
	}

	js, err := s.nc.JetStream()
	if err != nil {
		s.logger.Error("jetstream for reap publish", "err", err)
		return
	}

	subject := broker.FlowLifecycleSubject(s.username, f.FlowID)
	if _, err := js.Publish(subject, data,
		nats.MsgId(f.FlowID+"-reaped"),
	); err != nil {
		s.logger.Error("publish reaped lifecycle",
			"flow_id", f.FlowID, "err", err)
	}
}
