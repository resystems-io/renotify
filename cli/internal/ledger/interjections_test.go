// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package ledger

import (
	"testing"
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

func TestInsertInterjection(t *testing.T) {
	db := openTestDB(t)

	i := &payload.InterjectionCommand{
		FlowID:    "flow_inj_001",
		Action:    payload.InterjectionStop,
		Context:   "user requested abort",
		Timestamp: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC),
	}
	if err := db.InsertInterjection(testWC, i); err != nil {
		t.Fatal(err)
	}

	var action, ctx, username string
	err := db.db.QueryRow(
		"SELECT username, action, context FROM interjections WHERE flow_id = ?",
		"flow_inj_001",
	).Scan(&username, &action, &ctx)
	if err != nil {
		t.Fatal(err)
	}
	if username != "testuser" {
		t.Errorf("username = %q, want %q", username, "testuser")
	}
	if action != "stop" {
		t.Errorf("action = %q, want %q", action, "stop")
	}
	if ctx != "user requested abort" {
		t.Errorf("context = %q, want %q", ctx, "user requested abort")
	}
}

func TestInsertInterjection_Dedup(t *testing.T) {
	db := openTestDB(t)

	ts := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	i := &payload.InterjectionCommand{
		FlowID:    "flow_inj_dup",
		Action:    payload.InterjectionNote,
		Context:   "first",
		Timestamp: ts,
	}
	if err := db.InsertInterjection(testWC, i); err != nil {
		t.Fatal(err)
	}
	// Same (flow_id, timestamp) — should be no-op.
	i.Context = "second"
	if err := db.InsertInterjection(testWC, i); err != nil {
		t.Fatal(err)
	}

	var count int
	db.db.QueryRow(
		"SELECT COUNT(*) FROM interjections WHERE flow_id = ?",
		"flow_inj_dup",
	).Scan(&count)
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestInsertInterjection_AllActions(t *testing.T) {
	db := openTestDB(t)

	base := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	for idx, action := range []payload.InterjectionAction{
		payload.InterjectionStop,
		payload.InterjectionPause,
		payload.InterjectionNote,
	} {
		i := &payload.InterjectionCommand{
			FlowID:    "flow_inj_all",
			Action:    action,
			Timestamp: base.Add(time.Duration(idx) * time.Second),
		}
		if err := db.InsertInterjection(testWC, i); err != nil {
			t.Fatalf("action %q: %v", action, err)
		}
	}

	var count int
	db.db.QueryRow(
		"SELECT COUNT(*) FROM interjections WHERE flow_id = ?",
		"flow_inj_all",
	).Scan(&count)
	if count != 3 {
		t.Errorf("count = %d, want 3", count)
	}
}
