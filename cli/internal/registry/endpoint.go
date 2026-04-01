package registry

import (
	"encoding/json"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/ledger"
	"go.resystems.io/renotify/internal/payload"
)

// ActiveFlowsResult is the response payload for the svc.flows
// Core NATS Request-Reply endpoint.
type ActiveFlowsResult struct {
	Flows []payload.FlowLifecycleEvent `json:"flows"`
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

	events := make([]payload.FlowLifecycleEvent, len(flows))
	for i, f := range flows {
		events[i] = payload.FlowLifecycleEvent{
			FlowID:      f.FlowID,
			DaemonID:    f.DaemonID,
			WorkspaceID: f.WorkspaceID,
			Status:      payload.FlowActive,
			Label:       f.Label,
			Metadata:    f.Metadata,
			Timestamp:   f.RegisteredAt,
		}
	}

	result := ActiveFlowsResult{Flows: events}
	data, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("svc.flows marshal", "err", err)
		msg.Respond([]byte(`{"flows":[]}`))
		return
	}

	msg.Respond(data)
}
