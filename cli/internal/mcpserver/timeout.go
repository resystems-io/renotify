package mcpserver

import (
	"time"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/payload"
)

// startTimeoutTimer starts a goroutine that publishes an
// ErrorResponse and failed lifecycle event if the ask timeout
// expires before a human responds. The response subscriber
// (started by handleAsk) already handles ErrorResponse via the
// code field probe and resolves the DecisionResource.
//
// The timer is cancelled if the decision resolves first (via
// the DecisionStore.Resolved channel).
func (s *Server) startTimeoutTimer(
	flowID, notificationID string,
	timeoutSec int,
) {
	resolved := s.decisions.Resolved(notificationID)
	if resolved == nil {
		return
	}

	go func() {
		select {
		case <-time.After(time.Duration(timeoutSec) * time.Second):
			// Timeout expired — publish ErrorResponse.
			s.logger.Info("ask timeout",
				"notification_id", notificationID,
				"flow_id", flowID,
				"timeout_sec", timeoutSec)

			now := time.Now().UTC()

			// Publish ErrorResponse on .response subject. The
			// response subscriber handles this and resolves the
			// DecisionResource with decided=true, no fields.
			errResp := &payload.ErrorResponse{
				CorrelationID: notificationID,
				Code:          "timeout",
				Message:       "ask timeout expired",
				Timestamp:     now,
			}
			broker.PublishJSON(s.js,
				broker.FlowResponseSubject(s.username, flowID),
				notificationID+"-timeout", errResp)

			// Publish failed lifecycle event.
			event := &payload.FlowLifecycleEvent{
				FlowID:    flowID,
				DaemonID:  s.daemonID,
				Status:    payload.FlowFailed,
				Timestamp: now,
			}
			broker.PublishJSON(s.js,
				broker.FlowLifecycleSubject(s.username, flowID),
				flowID+"-timeout", event)

		case <-resolved:
			// Decision arrived before timeout — nothing to do.
		}
	}()
}
