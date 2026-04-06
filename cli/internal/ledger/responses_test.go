// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package ledger

import (
	"testing"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

func TestInsertResponse(t *testing.T) {
	db := openTestDB(t)
	insertTestRequest(t, db, "ntf_RESP0001")

	accepted := true
	r := &payload.NotificationResponse{
		RequestID: "ntf_RESP0001",
		Accepted:  &accepted,
		Timestamp: time.Now().UTC(),
	}
	if err := db.InsertResponse(r); err != nil {
		t.Fatal(err)
	}

	var reqID string
	var acc bool
	err := db.db.QueryRow(
		"SELECT request_id, accepted FROM notification_responses WHERE request_id = ?",
		"ntf_RESP0001",
	).Scan(&reqID, &acc)
	if err != nil {
		t.Fatal(err)
	}
	if !acc {
		t.Error("accepted = false, want true")
	}
}

func TestInsertResponse_Dedup(t *testing.T) {
	db := openTestDB(t)
	insertTestRequest(t, db, "ntf_RDUP0001")

	accepted := true
	r := &payload.NotificationResponse{
		RequestID: "ntf_RDUP0001",
		Accepted:  &accepted,
		Timestamp: time.Now().UTC(),
	}
	if err := db.InsertResponse(r); err != nil {
		t.Fatal(err)
	}
	// Second insert — should be no-op.
	if err := db.InsertResponse(r); err != nil {
		t.Fatal(err)
	}

	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM notification_responses").Scan(&count)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestInsertResponse_NullFields(t *testing.T) {
	db := openTestDB(t)
	insertTestRequest(t, db, "ntf_RNUL0001")

	r := &payload.NotificationResponse{
		RequestID: "ntf_RNUL0001",
		Accepted:  nil,
		Action:    "",
		Text:      "",
		Timestamp: time.Now().UTC(),
	}
	if err := db.InsertResponse(r); err != nil {
		t.Fatal(err)
	}

	var accepted, action, text interface{}
	db.db.QueryRow(
		"SELECT accepted, action, text FROM notification_responses WHERE request_id = ?",
		"ntf_RNUL0001",
	).Scan(&accepted, &action, &text)

	if accepted != nil {
		t.Errorf("accepted = %v, want nil", accepted)
	}
	if action != nil {
		t.Errorf("action = %v, want nil", action)
	}
	if text != nil {
		t.Errorf("text = %v, want nil", text)
	}
}

func TestInsertResponse_BooleanAccepted(t *testing.T) {
	db := openTestDB(t)

	// Test both true and false.
	for _, tc := range []struct {
		id   string
		val  bool
		want int64
	}{
		{"ntf_BTRU0001", true, 1},
		{"ntf_BFAL0001", false, 0},
	} {
		insertTestRequest(t, db, tc.id)
		r := &payload.NotificationResponse{
			RequestID: tc.id,
			Accepted:  &tc.val,
			Timestamp: time.Now().UTC(),
		}
		if err := db.InsertResponse(r); err != nil {
			t.Fatal(err)
		}

		var acc int64
		db.db.QueryRow(
			"SELECT accepted FROM notification_responses WHERE request_id = ?",
			tc.id,
		).Scan(&acc)
		if acc != tc.want {
			t.Errorf("%s: accepted = %d, want %d", tc.id, acc, tc.want)
		}
	}
}

// insertTestRequest is a helper that inserts a minimal request
// to satisfy the foreign key constraint on notification_responses.
func insertTestRequest(t *testing.T, db *DB, id string) {
	t.Helper()
	r := &payload.NotificationRequest{
		ID:            id,
		FlowID:        "flow_test",
		DaemonID:      "dmn_test",
		WorkspaceID:   "ws_test",
		Title:         "Test",
		ResponseTypes: []payload.ResponseType{payload.ResponseBoolean},
		Priority:      payload.PriorityNormal,
		Source:        "test",
		Timestamp:     time.Now().UTC(),
	}
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}
}
