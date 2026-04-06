// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package ledger

import (
	"testing"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

func TestQueryHistory_Empty(t *testing.T) {
	db := openTestDB(t)

	result, err := db.QueryHistory(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
	if len(result.Records) != 0 {
		t.Errorf("records = %d, want 0", len(result.Records))
	}
}

func TestQueryHistory_WithResponse(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_HIST0001", "flow_h1", "ws_h1", ts)

	accepted := true
	db.InsertResponse(&payload.NotificationResponse{
		RequestID: "ntf_HIST0001",
		Accepted:  &accepted,
		Timestamp: ts.Add(time.Minute),
	})

	result, err := db.QueryHistory(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("total = %d, want 1", result.Total)
	}
	rec := result.Records[0]
	if rec.Username != "testuser" {
		t.Errorf("username = %q, want %q", rec.Username, "testuser")
	}
	if rec.Request.ID != "ntf_HIST0001" {
		t.Errorf("request.id = %q", rec.Request.ID)
	}
	if rec.Response == nil {
		t.Fatal("response should not be nil")
	}
	if rec.Response.Accepted == nil || !*rec.Response.Accepted {
		t.Error("response.accepted should be true")
	}
}

func TestQueryHistory_WithoutResponse(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_NRSP0001", "flow_h2", "ws_h2", ts)

	result, err := db.QueryHistory(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Fatalf("total = %d, want 1", result.Total)
	}
	if result.Records[0].Response != nil {
		t.Error("response should be nil when no response exists")
	}
}

func TestQueryHistory_FilterWorkspace(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_FW01", "flow_fw1", "ws_alpha", ts)
	insertHistoryRequest(t, db, "ntf_FW02", "flow_fw2", "ws_beta", ts.Add(time.Second))

	result, err := db.QueryHistory(HistoryQuery{
		WorkspaceID: "ws_alpha",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
	if result.Records[0].Request.WorkspaceID != "ws_alpha" {
		t.Errorf("workspace = %q", result.Records[0].Request.WorkspaceID)
	}
}

func TestQueryHistory_FilterFlow(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_FF01", "flow_target", "ws_001", ts)
	insertHistoryRequest(t, db, "ntf_FF02", "flow_other", "ws_001", ts.Add(time.Second))

	result, err := db.QueryHistory(HistoryQuery{
		FlowID: "flow_target",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
}

func TestQueryHistory_FilterSince(t *testing.T) {
	db := openTestDB(t)

	ts1 := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_FS01", "flow_001", "ws_001", ts1)
	insertHistoryRequest(t, db, "ntf_FS02", "flow_001", "ws_001", ts2)

	since := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	result, err := db.QueryHistory(HistoryQuery{Since: &since})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
	if result.Records[0].Request.ID != "ntf_FS02" {
		t.Errorf("id = %q, want ntf_FS02", result.Records[0].Request.ID)
	}
}

func TestQueryHistory_FilterUntil(t *testing.T) {
	db := openTestDB(t)

	ts1 := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_FU01", "flow_001", "ws_001", ts1)
	insertHistoryRequest(t, db, "ntf_FU02", "flow_001", "ws_001", ts2)

	until := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	result, err := db.QueryHistory(HistoryQuery{Until: &until})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 {
		t.Errorf("total = %d, want 1", result.Total)
	}
	if result.Records[0].Request.ID != "ntf_FU01" {
		t.Errorf("id = %q, want ntf_FU01", result.Records[0].Request.ID)
	}
}

func TestQueryHistory_Pagination(t *testing.T) {
	db := openTestDB(t)

	base := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	for i := range 5 {
		id := "ntf_PG0" + string(rune('A'+i))
		insertHistoryRequest(t, db, id, "flow_pg", "ws_001",
			base.Add(time.Duration(i)*time.Minute))
	}

	// Page 1: limit=2, offset=0.
	result, err := db.QueryHistory(HistoryQuery{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 5 {
		t.Errorf("total = %d, want 5", result.Total)
	}
	if len(result.Records) != 2 {
		t.Errorf("records = %d, want 2", len(result.Records))
	}

	// Page 2: limit=2, offset=2.
	result, err = db.QueryHistory(HistoryQuery{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 5 {
		t.Errorf("total = %d, want 5", result.Total)
	}
	if len(result.Records) != 2 {
		t.Errorf("records = %d, want 2", len(result.Records))
	}

	// Last page: limit=2, offset=4.
	result, err = db.QueryHistory(HistoryQuery{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 {
		t.Errorf("records = %d, want 1", len(result.Records))
	}
}

func TestQueryHistory_DefaultLimit(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_DL01", "flow_001", "ws_001", ts)
	insertHistoryRequest(t, db, "ntf_DL02", "flow_001", "ws_001",
		ts.Add(time.Minute))

	result, err := db.QueryHistory(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Errorf("records = %d, want 2", len(result.Records))
	}
}

func TestQueryHistory_OrderDescending(t *testing.T) {
	db := openTestDB(t)

	ts1 := time.Date(2026, 3, 31, 8, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)
	insertHistoryRequest(t, db, "ntf_ORD1", "flow_001", "ws_001", ts1)
	insertHistoryRequest(t, db, "ntf_ORD2", "flow_001", "ws_001", ts2)

	result, err := db.QueryHistory(HistoryQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 2 {
		t.Fatalf("records = %d, want 2", len(result.Records))
	}
	// Most recent first.
	if result.Records[0].Request.ID != "ntf_ORD2" {
		t.Errorf("first = %q, want ntf_ORD2 (most recent)",
			result.Records[0].Request.ID)
	}
	if result.Records[1].Request.ID != "ntf_ORD1" {
		t.Errorf("second = %q, want ntf_ORD1",
			result.Records[1].Request.ID)
	}
}

// insertHistoryRequest is a test helper that inserts a minimal
// notification request.
func insertHistoryRequest(
	t *testing.T, db *DB,
	id, flowID, wsID string,
	ts time.Time,
) {
	t.Helper()
	r := &payload.NotificationRequest{
		ID:            id,
		FlowID:        flowID,
		DaemonID:      "dmn_hist",
		WorkspaceID:   wsID,
		Title:         "Test " + id,
		ResponseTypes: []payload.ResponseType{payload.ResponseNone},
		Priority:      payload.PriorityNormal,
		Source:        "test",
		Timestamp:     ts,
	}
	if err := db.InsertRequest(testWC, r); err != nil {
		t.Fatal(err)
	}
}
