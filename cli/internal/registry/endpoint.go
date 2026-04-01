package registry

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/ledger"
)

// ActiveFlowEntry is a single entry in the ActiveFlowsResult.
// Includes fields needed by the CLI flows command (display name,
// last activity for TTL computation).
type ActiveFlowEntry struct {
	FlowID                string    `json:"flow_id"`
	DaemonID              string    `json:"daemon_id"`
	WorkspaceID           string    `json:"workspace_id"`
	DisplayName           string    `json:"display_name,omitempty"`
	Label                 string    `json:"label,omitempty"`
	RegisteredAt          time.Time `json:"registered_at"`
	LastActivityTimestamp time.Time `json:"last_activity_timestamp"`
}

// ActiveFlowsResult is the response payload for the svc.flows
// Core NATS Request-Reply endpoint.
type ActiveFlowsResult struct {
	Flows []ActiveFlowEntry `json:"flows"`
}

// subscribeFlowsEndpoint subscribes to the svc.flows subject
// and handles incoming ActiveFlowsQuery requests.
func (s *Service) subscribeFlowsEndpoint() (*nats.Subscription, error) {
	subject := broker.ServiceFlowsSubject(s.username)
	sub, err := s.nc.Subscribe(subject, s.handleFlowsQuery)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.flows endpoint ready", "subject", subject)
	return sub, nil
}

// handleFlowsQuery processes a single svc.flows request.
func (s *Service) handleFlowsQuery(msg *nats.Msg) {
	var query ledger.ActiveFlowsQuery
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &query); err != nil {
			s.logger.Error("svc.flows unmarshal", "err", err)
			msg.Respond([]byte(`{"flows":[]}`))
			return
		}
	}

	flows, err := s.dbFunc().ListActiveFlows(query)
	if err != nil {
		s.logger.Error("svc.flows query", "err", err)
		msg.Respond([]byte(`{"flows":[]}`))
		return
	}

	entries := make([]ActiveFlowEntry, len(flows))
	for i, f := range flows {
		entries[i] = ActiveFlowEntry{
			FlowID:                f.FlowID,
			DaemonID:              f.DaemonID,
			WorkspaceID:           f.WorkspaceID,
			DisplayName:           f.DisplayName,
			Label:                 f.Label,
			RegisteredAt:          f.RegisteredAt,
			LastActivityTimestamp: f.LastActivityTimestamp,
		}
	}

	result := ActiveFlowsResult{Flows: entries}
	data, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("svc.flows marshal", "err", err)
		msg.Respond([]byte(`{"flows":[]}`))
		return
	}

	msg.Respond(data)
}
