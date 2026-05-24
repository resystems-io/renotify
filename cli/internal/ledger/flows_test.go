// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package ledger

import (
	"testing"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

func TestRegisterFlow(t *testing.T) {
	db := openTestDB(t)

	f := &ActiveFlow{
		FlowID:       "flow_reg_001",
		Username:     "testuser",
		DaemonID:     "dmn_001",
		WorkspaceID:  "ws_001",
		Label:        "build pipeline",
		Metadata:     map[string]string{"branch": "main"},
		RegisteredAt: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC),
	}
	if err := db.RegisterFlow(f); err != nil {
		t.Fatal(err)
	}

	// Verify active_flows row.
	var flowID, label, username string
	err := db.db.QueryRow(
		"SELECT flow_id, username, label FROM active_flows WHERE flow_id = ?",
		"flow_reg_001",
	).Scan(&flowID, &username, &label)
	if err != nil {
		t.Fatalf("active_flows: %v", err)
	}
	if username != "testuser" {
		t.Errorf("username = %q, want %q", username, "testuser")
	}
	if label != "build pipeline" {
		t.Errorf("label = %q, want %q", label, "build pipeline")
	}

	// Verify lifecycle event.
	var status string
	err = db.db.QueryRow(
		"SELECT status FROM flow_lifecycle_events WHERE flow_id = ?",
		"flow_reg_001",
	).Scan(&status)
	if err != nil {
		t.Fatalf("flow_lifecycle_events: %v", err)
	}
	if status != "active" {
		t.Errorf("status = %q, want %q", status, "active")
	}
}

func TestUpdateFlowActivity(t *testing.T) {
	db := openTestDB(t)

	ts1 := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	registerTestFlow(t, db, "flow_act_001", ts1)

	ts2 := ts1.Add(5 * time.Minute)
	if err := db.UpdateFlowActivity("flow_act_001", ts2); err != nil {
		t.Fatal(err)
	}

	var lastAct string
	db.db.QueryRow(
		"SELECT last_activity_timestamp FROM active_flows WHERE flow_id = ?",
		"flow_act_001",
	).Scan(&lastAct)

	want := ts2.UTC().Format(time.RFC3339)
	if lastAct != want {
		t.Errorf("last_activity = %q, want %q", lastAct, want)
	}
}

func TestRefreshFlow(t *testing.T) {
	db := openTestDB(t)

	ts1 := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	registerTestFlow(t, db, "flow_ref_001", ts1)

	ts2 := ts1.Add(5 * time.Minute)
	err := db.RefreshFlow("flow_ref_001", "updated label",
		map[string]string{"key": "val"}, ts2)
	if err != nil {
		t.Fatal(err)
	}

	// Verify active_flows updated.
	var label string
	db.db.QueryRow(
		"SELECT label FROM active_flows WHERE flow_id = ?",
		"flow_ref_001",
	).Scan(&label)
	if label != "updated label" {
		t.Errorf("label = %q, want %q", label, "updated label")
	}

	// Verify two lifecycle events (initial + refresh) and that
	// username propagated from active_flows to the event.
	var count int
	db.db.QueryRow(
		"SELECT COUNT(*) FROM flow_lifecycle_events WHERE flow_id = ? AND username = 'testuser'",
		"flow_ref_001",
	).Scan(&count)
	if count != 2 {
		t.Errorf("lifecycle event count = %d, want 2", count)
	}
}

func TestTerminateFlow(t *testing.T) {
	db := openTestDB(t)

	ts1 := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	registerTestFlow(t, db, "flow_term_001", ts1)

	ts2 := ts1.Add(10 * time.Minute)
	if err := db.TerminateFlow("flow_term_001", "completed", ts2); err != nil {
		t.Fatal(err)
	}

	// Active flow should be deleted.
	var count int
	db.db.QueryRow(
		"SELECT COUNT(*) FROM active_flows WHERE flow_id = ?",
		"flow_term_001",
	).Scan(&count)
	if count != 0 {
		t.Errorf("active_flows count = %d, want 0", count)
	}

	// Terminal lifecycle event should exist with username.
	var status, username string
	db.db.QueryRow(`
		SELECT status, username FROM flow_lifecycle_events
		WHERE flow_id = ? ORDER BY timestamp DESC LIMIT 1`,
		"flow_term_001",
	).Scan(&status, &username)
	if status != "completed" {
		t.Errorf("terminal status = %q, want %q", status, "completed")
	}
	if username != "testuser" {
		t.Errorf("terminal username = %q, want %q", username, "testuser")
	}
}

func TestTerminateFlow_NotFound(t *testing.T) {
	db := openTestDB(t)

	// Terminating a non-existent flow should not error.
	err := db.TerminateFlow("flow_ghost", "failed", time.Now().UTC())
	if err != nil {
		t.Fatalf("TerminateFlow on missing flow should not error: %v", err)
	}
}

func TestListActiveFlows_All(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	registerTestFlow(t, db, "flow_ls_001", ts)
	registerTestFlowWS(t, db, "flow_ls_002", "ws_other", ts.Add(time.Minute))

	flows, err := db.ListActiveFlows(ActiveFlowsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 2 {
		t.Errorf("len = %d, want 2", len(flows))
	}
}

func TestListActiveFlows_ByWorkspace(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	registerTestFlow(t, db, "flow_ws_001", ts)
	registerTestFlowWS(t, db, "flow_ws_002", "ws_other", ts.Add(time.Minute))

	flows, err := db.ListActiveFlows(ActiveFlowsQuery{
		WorkspaceID: "ws_001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Errorf("len = %d, want 1", len(flows))
	}
	if flows[0].FlowID != "flow_ws_001" {
		t.Errorf("flow_id = %q, want %q", flows[0].FlowID, "flow_ws_001")
	}
}

func TestListActiveFlows_ByDaemon(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	registerTestFlow(t, db, "flow_dmn_001", ts)

	flows, err := db.ListActiveFlows(ActiveFlowsQuery{
		DaemonID: "dmn_001",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Errorf("len = %d, want 1", len(flows))
	}

	flows, err = db.ListActiveFlows(ActiveFlowsQuery{
		DaemonID: "dmn_other",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 0 {
		t.Errorf("len = %d, want 0", len(flows))
	}
}

func TestDB_SearchActiveFlows(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)

	f1 := &ActiveFlow{
		FlowID:       "flow_srch_001",
		Username:     "testuser",
		DaemonID:     "dmn_001",
		WorkspaceID:  "ws_001",
		DisplayName:  "myproj",
		AbsPath:      "/home/test/myproj",
		RegisteredAt: ts,
	}
	if err := db.RegisterFlow(f1); err != nil {
		t.Fatal(err)
	}

	f2 := &ActiveFlow{
		FlowID:       "flow_srch_002",
		Username:     "testuser",
		DaemonID:     "dmn_001",
		WorkspaceID:  "ws_002",
		DisplayName:  "otherproj",
		AbsPath:      "/home/test/otherproj",
		RegisteredAt: ts.Add(time.Minute),
	}
	if err := db.RegisterFlow(f2); err != nil {
		t.Fatal(err)
	}

	// Search by WorkspaceName (display_name)
	flows, err := db.SearchActiveFlows(SearchFlowsQuery{Query: "myproj"})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("len = %d, want 1", len(flows))
	}
	if flows[0].FlowID != "flow_srch_001" {
		t.Errorf("flow_id = %q, want %q", flows[0].FlowID, "flow_srch_001")
	}

	// Search by WorkspacePath (abs_path)
	flows, err = db.SearchActiveFlows(SearchFlowsQuery{Query: "/home/test/otherproj"})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("len = %d, want 1", len(flows))
	}
	if flows[0].FlowID != "flow_srch_002" {
		t.Errorf("flow_id = %q, want %q", flows[0].FlowID, "flow_srch_002")
	}
}

func TestReapStaleFlows(t *testing.T) {
	db := openTestDB(t)

	// Old flow: activity 10 minutes ago.
	old := time.Now().UTC().Add(-10 * time.Minute)
	registerTestFlowAt(t, db, "flow_stale", old)

	// Fresh flow: activity just now.
	fresh := time.Now().UTC()
	registerTestFlowAt(t, db, "flow_fresh", fresh)

	// Reap with 5-minute grace period.
	stale, err := db.ReapStaleFlows(5 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 {
		t.Fatalf("stale count = %d, want 1", len(stale))
	}
	if stale[0].FlowID != "flow_stale" {
		t.Errorf("stale flow = %q, want %q", stale[0].FlowID, "flow_stale")
	}
}

func TestInsertLifecycleEvent_Dedup(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	e := &payload.FlowLifecycleEvent{
		FlowID:      "flow_lce_001",
		DaemonID:    "dmn_001",
		WorkspaceID: "ws_001",
		Status:      payload.FlowActive,
		Timestamp:   ts,
	}
	if err := db.InsertLifecycleEvent(testWC, e); err != nil {
		t.Fatal(err)
	}
	// Same (flow_id, timestamp) — should be no-op.
	if err := db.InsertLifecycleEvent(testWC, e); err != nil {
		t.Fatal(err)
	}

	var count int
	db.db.QueryRow(
		"SELECT COUNT(*) FROM flow_lifecycle_events WHERE flow_id = ?",
		"flow_lce_001",
	).Scan(&count)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

// Test helpers.

func registerTestFlow(t *testing.T, db *DB, flowID string, ts time.Time) {
	t.Helper()
	registerTestFlowWS(t, db, flowID, "ws_001", ts)
}

func registerTestFlowWS(t *testing.T, db *DB, flowID, wsID string, ts time.Time) {
	t.Helper()
	f := &ActiveFlow{
		FlowID:       flowID,
		Username:     "testuser",
		DaemonID:     "dmn_001",
		WorkspaceID:  wsID,
		RegisteredAt: ts,
	}
	if err := db.RegisterFlow(f); err != nil {
		t.Fatal(err)
	}
}

func registerTestFlowAt(t *testing.T, db *DB, flowID string, ts time.Time) {
	t.Helper()
	f := &ActiveFlow{
		FlowID:       flowID,
		Username:     "testuser",
		DaemonID:     "dmn_001",
		WorkspaceID:  "ws_001",
		RegisteredAt: ts,
	}
	if err := db.RegisterFlow(f); err != nil {
		t.Fatal(err)
	}
}
