// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package payload

import (
	"encoding/json"
	"testing"
	"time"
)

var testTime = time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)

func TestNotificationRequest_RoundTrip(t *testing.T) {
	req := NotificationRequest{
		ID:            "ntf_ABC123",
		FlowID:        "fl_DEF456",
		DaemonID:      "dn_GHI789",
		WorkspaceID:   "ws_JKL012",
		Title:         "Deploy?",
		Body:          "Ready to deploy v1.2",
		ResponseTypes: []ResponseType{ResponseBoolean, ResponseText},
		Priority:      PriorityHigh,
		Source:        "ci-pipeline",
		Actions:       []string{"approve", "reject"},
		TimeoutSec:    300,
		Timestamp:     testTime,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got NotificationRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.ID != req.ID {
		t.Errorf("ID = %q, want %q", got.ID, req.ID)
	}
	if got.Title != req.Title {
		t.Errorf("Title = %q, want %q", got.Title, req.Title)
	}
	if len(got.ResponseTypes) != 2 {
		t.Errorf("ResponseTypes len = %d, want 2", len(got.ResponseTypes))
	}
	if got.Priority != PriorityHigh {
		t.Errorf("Priority = %q, want %q", got.Priority, PriorityHigh)
	}
	if got.TimeoutSec != 300 {
		t.Errorf("TimeoutSec = %d, want 300", got.TimeoutSec)
	}
}

func TestNotificationRequest_OmitEmpty(t *testing.T) {
	req := NotificationRequest{
		ID:            "ntf_ABC123",
		FlowID:        "fl_DEF456",
		DaemonID:      "dn_GHI789",
		WorkspaceID:   "ws_JKL012",
		Title:         "Alert",
		ResponseTypes: []ResponseType{ResponseNone},
		Priority:      PriorityNormal,
		Source:        "test",
		Timestamp:     testTime,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := string(data)
	for _, field := range []string{"body", "actions", "timeout_sec", "workspace_name"} {
		if testing.Verbose() {
			t.Logf("JSON: %s", raw)
		}
		if json.Valid(data) {
			var m map[string]any
			json.Unmarshal(data, &m)
			if _, ok := m[field]; ok {
				t.Errorf("expected %q to be omitted when empty", field)
			}
		}
	}
}

func TestNotificationResponse_RoundTrip(t *testing.T) {
	accepted := true
	resp := NotificationResponse{
		RequestID: "ntf_ABC123",
		Accepted:  &accepted,
		Action:    "approve",
		Text:      "Looks good",
		Timestamp: testTime,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got NotificationResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.RequestID != resp.RequestID {
		t.Errorf("RequestID = %q, want %q", got.RequestID, resp.RequestID)
	}
	if got.Accepted == nil || *got.Accepted != true {
		t.Error("Accepted should be true")
	}
	if got.Action != "approve" {
		t.Errorf("Action = %q, want %q", got.Action, "approve")
	}
}

func TestFlowLifecycleEvent_RoundTrip(t *testing.T) {
	ev := FlowLifecycleEvent{
		FlowID:      "fl_DEF456",
		DaemonID:    "dn_GHI789",
		WorkspaceID: "ws_JKL012",
		Status:      FlowActive,
		Label:       "deploy pipeline",
		Metadata:    map[string]string{"branch": "main"},
		Timestamp:   testTime,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got FlowLifecycleEvent
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Status != FlowActive {
		t.Errorf("Status = %q, want %q", got.Status, FlowActive)
	}
	if got.Metadata["branch"] != "main" {
		t.Errorf("Metadata[branch] = %q, want %q", got.Metadata["branch"], "main")
	}
}

func TestErrorResponse_RoundTrip(t *testing.T) {
	er := ErrorResponse{
		CorrelationID: "ntf_ABC123",
		Code:          "timeout",
		Message:       "no response within 300s",
		Timestamp:     testTime,
	}

	data, err := json.Marshal(er)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got ErrorResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Code != "timeout" {
		t.Errorf("Code = %q, want %q", got.Code, "timeout")
	}
}

func TestInterjectionCommand_RoundTrip(t *testing.T) {
	cmd := InterjectionCommand{
		FlowID:    "fl_DEF456",
		Action:    InterjectionStop,
		Context:   "user requested abort",
		Timestamp: testTime,
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got InterjectionCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Action != InterjectionStop {
		t.Errorf("Action = %q, want %q", got.Action, InterjectionStop)
	}
}

func TestDecisionResource_RoundTrip(t *testing.T) {
	accepted := false
	dr := DecisionResource{
		RequestID: "ntf_ABC123",
		Decided:   true,
		Accepted:  &accepted,
		Timestamp: testTime,
	}

	data, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got DecisionResource
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !got.Decided {
		t.Error("Decided should be true")
	}
	if got.Accepted == nil || *got.Accepted != false {
		t.Error("Accepted should be false")
	}
}

func TestSnakeCaseKeys(t *testing.T) {
	req := NotificationRequest{
		ID:            "ntf_X",
		FlowID:        "fl_X",
		DaemonID:      "dn_X",
		WorkspaceID:   "ws_X",
		Title:         "T",
		ResponseTypes: []ResponseType{ResponseNone},
		Priority:      PriorityNormal,
		Source:        "test",
		Timestamp:     testTime,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}

	for _, key := range []string{"id", "flow_id", "daemon_id", "workspace_id", "response_types", "timestamp"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected snake_case key %q in JSON output", key)
		}
	}
}
