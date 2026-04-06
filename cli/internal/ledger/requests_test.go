// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package ledger

import (
	"testing"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

var testWC = WriteContext{Username: "testuser"}

func TestInsertRequest(t *testing.T) {
	db := openTestDB(t)

	r := &payload.NotificationRequest{
		ID:            "ntf_TEST0001",
		FlowID:        "flow_001",
		DaemonID:      "dmn_001",
		WorkspaceID:   "ws_001",
		Title:         "Deploy?",
		Body:          "Deploy to production",
		ResponseTypes: []payload.ResponseType{payload.ResponseBoolean},
		Priority:      payload.PriorityHigh,
		Source:        "cli",
		Actions:       []string{"Approve", "Reject"},
		TimeoutSec:    300,
		Timestamp:     time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC),
	}
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}

	// Verify via raw SQL.
	var id, title, priority, username string
	err := db.db.QueryRow(
		"SELECT id, username, title, priority FROM notification_requests WHERE id = ?",
		"ntf_TEST0001",
	).Scan(&id, &username, &title, &priority)
	if err != nil {
		t.Fatal(err)
	}
	if username != "testuser" {
		t.Errorf("username = %q, want %q", username, "testuser")
	}
	if title != "Deploy?" {
		t.Errorf("title = %q, want %q", title, "Deploy?")
	}
	if priority != "high" {
		t.Errorf("priority = %q, want %q", priority, "high")
	}
}

func TestInsertRequest_Dedup(t *testing.T) {
	db := openTestDB(t)

	r := &payload.NotificationRequest{
		ID:            "ntf_DEDUP001",
		FlowID:        "flow_001",
		DaemonID:      "dmn_001",
		WorkspaceID:   "ws_001",
		Title:         "First",
		ResponseTypes: []payload.ResponseType{payload.ResponseNone},
		Priority:      payload.PriorityNormal,
		Source:        "cli",
		Timestamp:     time.Now().UTC(),
	}
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}

	// Second insert with same ID — should be silent no-op.
	r.Title = "Second"
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}

	// Original title should be preserved.
	var title string
	db.db.QueryRow(
		"SELECT title FROM notification_requests WHERE id = ?",
		"ntf_DEDUP001",
	).Scan(&title)
	if title != "First" {
		t.Errorf("title = %q, want %q (dedup should keep first)", title, "First")
	}

	// Only one row should exist.
	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM notification_requests").Scan(&count)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestInsertRequest_JSONFields(t *testing.T) {
	db := openTestDB(t)

	r := &payload.NotificationRequest{
		ID:            "ntf_JSON0001",
		FlowID:        "flow_001",
		DaemonID:      "dmn_001",
		WorkspaceID:   "ws_001",
		Title:         "Choose",
		ResponseTypes: []payload.ResponseType{payload.ResponseChoice, payload.ResponseText},
		Priority:      payload.PriorityLow,
		Source:        "mcp",
		Actions:       []string{"A", "B", "C"},
		Timestamp:     time.Now().UTC(),
	}
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}

	// Verify JSON stored correctly.
	var respTypes, actions string
	db.db.QueryRow(
		"SELECT response_types, actions FROM notification_requests WHERE id = ?",
		"ntf_JSON0001",
	).Scan(&respTypes, &actions)

	if respTypes != `["choice","text"]` {
		t.Errorf("response_types = %q, want %q", respTypes, `["choice","text"]`)
	}
	if actions != `["A","B","C"]` {
		t.Errorf("actions = %q, want %q", actions, `["A","B","C"]`)
	}
}

func TestInsertRequest_NullOptionals(t *testing.T) {
	db := openTestDB(t)

	r := &payload.NotificationRequest{
		ID:            "ntf_NULL0001",
		FlowID:        "flow_001",
		DaemonID:      "dmn_001",
		WorkspaceID:   "ws_001",
		Title:         "Simple",
		Body:          "", // should be NULL
		ResponseTypes: []payload.ResponseType{payload.ResponseNone},
		Priority:      payload.PriorityNormal,
		Source:        "cli",
		Actions:       nil, // should be NULL
		TimeoutSec:    0,   // should be NULL
		Timestamp:     time.Now().UTC(),
	}
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}

	var body, actions, timeout interface{}
	db.db.QueryRow(
		"SELECT body, actions, timeout_sec FROM notification_requests WHERE id = ?",
		"ntf_NULL0001",
	).Scan(&body, &actions, &timeout)

	if body != nil {
		t.Errorf("body = %v, want nil (NULL)", body)
	}
	if actions != nil {
		t.Errorf("actions = %v, want nil (NULL)", actions)
	}
	if timeout != nil {
		t.Errorf("timeout_sec = %v, want nil (NULL)", timeout)
	}
}

func TestCountRecentRequests(t *testing.T) {
	db := openTestDB(t)

	now := time.Now().UTC()

	// Insert 3 recent requests for flow_001.
	for i := range 3 {
		r := &payload.NotificationRequest{
			ID:            "ntf_COUNT" + string(rune('A'+i)),
			FlowID:        "flow_001",
			DaemonID:      "dmn_001",
			WorkspaceID:   "ws_001",
			Title:         "Test",
			ResponseTypes: []payload.ResponseType{payload.ResponseNone},
			Priority:      payload.PriorityNormal,
			Source:        "cli",
			Timestamp:     now.Add(-time.Duration(i) * time.Second),
		}
		if err := db.InsertRequest(testWC, r); err != nil {
			t.Fatal(err)
		}
	}

	// Insert 1 old request (2 hours ago).
	r := &payload.NotificationRequest{
		ID:            "ntf_OLD00001",
		FlowID:        "flow_001",
		DaemonID:      "dmn_001",
		WorkspaceID:   "ws_001",
		Title:         "Old",
		ResponseTypes: []payload.ResponseType{payload.ResponseNone},
		Priority:      payload.PriorityNormal,
		Source:        "cli",
		Timestamp:     now.Add(-2 * time.Hour),
	}
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}

	count, err := db.CountRecentRequests("flow_001", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Errorf("count = %d, want 3 (old request excluded)", count)
	}
}
