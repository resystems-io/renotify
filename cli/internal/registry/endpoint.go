// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package registry

import (
	"encoding/json"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/ledger"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

// subscribeFlowsEndpoint subscribes to the svc.flows subject
// and handles incoming FlowsQuery requests.
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
	var query statesvc.FlowsQuery
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &query); err != nil {
			s.logger.Error("svc.flows unmarshal", "err", err)
			msg.Respond([]byte(`{"flows":[]}`))
			return
		}
	}

	flows, err := s.dbFunc().ListActiveFlows(ledger.ActiveFlowsQuery{
		FlowID:      query.FlowID,
		DaemonID:    query.DaemonID,
		WorkspaceID: query.WorkspaceID,
	})
	if err != nil {
		s.logger.Error("svc.flows query", "err", err)
		msg.Respond([]byte(`{"flows":[]}`))
		return
	}

	entries := make([]statesvc.FlowEntry, len(flows))
	for i, f := range flows {
		entries[i] = statesvc.FlowEntry{
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

	result := statesvc.FlowsResult{Flows: entries}
	data, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("svc.flows marshal", "err", err)
		msg.Respond([]byte(`{"flows":[]}`))
		return
	}

	msg.Respond(data)
}

// --- Search endpoint ---

// subscribeSearchFlowsEndpoint subscribes to the svc.flows.search subject
// and handles incoming SearchFlowsQuery requests.
func (s *Service) subscribeSearchFlowsEndpoint() (*nats.Subscription, error) {
	subject := broker.ServiceSearchFlowsSubject(s.username)
	sub, err := s.nc.Subscribe(subject, s.handleSearchFlowsQuery)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.flows.search endpoint ready", "subject", subject)
	return sub, nil
}

// handleSearchFlowsQuery processes a single svc.flows.search request.
func (s *Service) handleSearchFlowsQuery(msg *nats.Msg) {
	var query statesvc.SearchFlowsQuery
	if len(msg.Data) > 0 {
		if err := json.Unmarshal(msg.Data, &query); err != nil {
			s.logger.Error("svc.flows.search unmarshal", "err", err)
			msg.Respond([]byte(`{"flow_ids":[]}`))
			return
		}
	}

	flows, err := s.dbFunc().SearchActiveFlows(ledger.SearchFlowsQuery{
		Query: query.Workspace,
	})
	if err != nil {
		s.logger.Error("svc.flows.search query", "err", err)
		msg.Respond([]byte(`{"flow_ids":[]}`))
		return
	}

	flowIDs := make([]string, len(flows))
	for i, f := range flows {
		flowIDs[i] = f.FlowID
	}
	if len(flowIDs) == 0 {
		// Ensure JSON marshals as [] rather than null
		flowIDs = []string{}
	}

	result := statesvc.SearchFlowsResult{FlowIDs: flowIDs}
	data, err := json.Marshal(result)
	if err != nil {
		s.logger.Error("svc.flows.search marshal", "err", err)
		msg.Respond([]byte(`{"flow_ids":[]}`))
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
	var req statesvc.HistoryQueryRequest
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

	// Convert ledger records to statesvc wire types.
	records := make([]statesvc.HistoryRecord, len(result.Records))
	for i, r := range result.Records {
		records[i] = statesvc.HistoryRecord{
			Type:          r.Type,
			Username:      r.Username,
			FlowLabel:     r.FlowLabel,
			WorkspaceName: r.WorkspaceName,
			WorkspacePath: r.WorkspacePath,
			Request:       r.Request,
			Response:      r.Response,
			Lifecycle:     r.Lifecycle,
		}
	}

	out := statesvc.HistoryQueryResult{
		Records: records,
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
