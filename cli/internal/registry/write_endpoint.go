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

var okResult = []byte(`{"ok":true}`)

func writeError(msg *nats.Msg, text string) {
	data, _ := json.Marshal(statesvc.WriteResult{Error: text})
	msg.Respond(data)
}

// subscribeInsertRequestEndpoint subscribes to the
// svc.insert-request subject (C-17).
func (s *Service) subscribeInsertRequestEndpoint() (
	*nats.Subscription, error,
) {
	subject := broker.ServiceInsertRequestSubject(s.username)
	sub, err := s.nc.Subscribe(subject, s.handleInsertRequest)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.insert-request endpoint ready",
		"subject", subject)
	return sub, nil
}

func (s *Service) handleInsertRequest(msg *nats.Msg) {
	var cmd statesvc.InsertRequestCmd
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		writeError(msg, "unmarshal: "+err.Error())
		return
	}
	s.dbFunc().InsertRequest(
		ledger.WriteContext{
			Username:      cmd.Username,
			FlowLabel:     cmd.FlowLabel,
			WorkspaceName: cmd.WorkspaceName,
			WorkspacePath: cmd.WorkspacePath,
		}, &cmd.Request)
	msg.Respond(okResult)
}

// subscribeInsertResponseEndpoint subscribes to the
// svc.insert-response subject (C-17).
func (s *Service) subscribeInsertResponseEndpoint() (
	*nats.Subscription, error,
) {
	subject := broker.ServiceInsertResponseSubject(s.username)
	sub, err := s.nc.Subscribe(subject, s.handleInsertResponse)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.insert-response endpoint ready",
		"subject", subject)
	return sub, nil
}

func (s *Service) handleInsertResponse(msg *nats.Msg) {
	var cmd statesvc.InsertResponseCmd
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		writeError(msg, "unmarshal: "+err.Error())
		return
	}
	s.dbFunc().InsertResponse(&cmd.Response)
	msg.Respond(okResult)
}

// subscribeInsertInterjectionEndpoint subscribes to the
// svc.insert-interjection subject (C-17).
func (s *Service) subscribeInsertInterjectionEndpoint() (
	*nats.Subscription, error,
) {
	subject := broker.ServiceInsertInterjectionSubject(s.username)
	sub, err := s.nc.Subscribe(subject,
		s.handleInsertInterjection)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.insert-interjection endpoint ready",
		"subject", subject)
	return sub, nil
}

func (s *Service) handleInsertInterjection(msg *nats.Msg) {
	var cmd statesvc.InsertInterjectionCmd
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		writeError(msg, "unmarshal: "+err.Error())
		return
	}
	s.dbFunc().InsertInterjection(
		ledger.WriteContext{Username: cmd.Username},
		&cmd.Interjection)
	msg.Respond(okResult)
}

// subscribeUpdateActivityEndpoint subscribes to the
// svc.update-activity subject (C-17).
func (s *Service) subscribeUpdateActivityEndpoint() (
	*nats.Subscription, error,
) {
	subject := broker.ServiceUpdateActivitySubject(s.username)
	sub, err := s.nc.Subscribe(subject, s.handleUpdateActivity)
	if err != nil {
		return nil, err
	}
	s.logger.Info("svc.update-activity endpoint ready",
		"subject", subject)
	return sub, nil
}

func (s *Service) handleUpdateActivity(msg *nats.Msg) {
	var cmd statesvc.UpdateActivityCmd
	if err := json.Unmarshal(msg.Data, &cmd); err != nil {
		writeError(msg, "unmarshal: "+err.Error())
		return
	}
	s.dbFunc().UpdateFlowActivity(cmd.FlowID, cmd.Timestamp)
	msg.Respond(okResult)
}
