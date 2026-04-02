package registry

import (
	"encoding/json"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/ledger"
)

// --- History endpoint wire types (C-09) ---

// HistoryQueryRequest is the inbound query for the svc.history
// Core NATS Request-Reply endpoint. All fields are optional
// filters; when omitted, the daemon returns the most recent
// records up to the default limit.
type HistoryQueryRequest struct {
	WorkspaceID string     `json:"workspace_id,omitempty"`
	FlowID      string     `json:"flow_id,omitempty"`
	Since       *time.Time `json:"since,omitempty"`
	Until       *time.Time `json:"until,omitempty"`
	Limit       int        `json:"limit,omitempty"`
	Offset      int        `json:"offset,omitempty"`
}

// HistoryQueryResult is the response payload for the svc.history
// Core NATS Request-Reply endpoint.
type HistoryQueryResult struct {
	Records []ledger.HistoryRecord `json:"records"`
	Total   int                    `json:"total"`
}

// --- Active flows endpoint wire types ---

// ActiveFlowEntry is a single entry in the ActiveFlowsResult.
// Includes fields needed by the CLI flows command (display name,
// last activity for TTL computation).
type ActiveFlowEntry struct {
	FlowID                string            `json:"flow_id"`
	DaemonID              string            `json:"daemon_id"`
	WorkspaceID           string            `json:"workspace_id"`
	DisplayName           string            `json:"display_name,omitempty"`
	AbsPath               string            `json:"abs_path,omitempty"`
	Label                 string            `json:"label,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	RegisteredAt          time.Time         `json:"registered_at"`
	LastActivityTimestamp time.Time         `json:"last_activity_timestamp"`
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
			AbsPath:               f.AbsPath,
			Label:                 f.Label,
			Metadata:              f.Metadata,
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

// --- History endpoint (C-09) ---

// subscribeHistoryEndpoint subscribes to the svc.history subject
// and handles incoming HistoryQueryRequest messages.
func (s *Service) subscribeHistoryEndpoint() (*nats.Subscription, error) {
	subject := broker.ServiceHistorySubject(s.username)
	sub, err := s.nc.Subscribe(subject, s.handleHistoryQuery)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.history endpoint ready", "subject", subject)
	return sub, nil
}

var emptyHistoryResult = []byte(`{"records":[],"total":0}`)

// handleHistoryQuery processes a single svc.history request.
func (s *Service) handleHistoryQuery(msg *nats.Msg) {
	var req HistoryQueryRequest
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			s.logger.Error("svc.history unmarshal", "err", err)
			msg.Respond(emptyHistoryResult)
			return
		}
	}

	query := ledger.HistoryQuery{
		WorkspaceID: req.WorkspaceID,
		FlowID:      req.FlowID,
		Since:       req.Since,
		Until:       req.Until,
		Limit:       req.Limit,
		Offset:      req.Offset,
	}

	result, err := s.dbFunc().QueryHistory(query)
	if err != nil {
		s.logger.Error("svc.history query", "err", err)
		msg.Respond(emptyHistoryResult)
		return
	}

	out := HistoryQueryResult{
		Records: result.Records,
		Total:   result.Total,
	}

	data, err := json.Marshal(out)
	if err != nil {
		s.logger.Error("svc.history marshal", "err", err)
		msg.Respond(emptyHistoryResult)
		return
	}

	msg.Respond(data)
}
