// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package registry_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/config"
	"go.resystems.io/renotify/cli/internal/heartbeat"
	"go.resystems.io/renotify/cli/internal/ledger"
	"go.resystems.io/renotify/cli/internal/payload"
	"go.resystems.io/renotify/cli/internal/presence"
	"go.resystems.io/renotify/cli/internal/registry"
	"go.resystems.io/renotify/cli/internal/state"
	"go.resystems.io/renotify/cli/internal/statesvc"
	"go.resystems.io/renotify/cli/internal/testutil"

	"log/slog"
)

const testUsername = "testuser"
const testDaemonID = "dn_TEST1234ABCDE"

// startTestNATS starts an embedded NATS server with JetStream
// and returns the server and a client connection. Mirrors the
// helpers in command/*_test.go.
func startTestNATS(t *testing.T) (*natsserver.Server, *nats.Conn) {
	t.Helper()

	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}

	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats not ready")
	}
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)

	// Create stream and consumers.
	logger := slog.Default()
	if err := broker.EnsureJetStream(
		t.Context(), nc, testUsername, nil,
		config.Default().JetStream, logger,
	); err != nil {
		t.Fatal(err)
	}

	return srv, nc
}

func openTestLedger(t *testing.T) *ledger.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := ledger.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newTestHeartbeat(t *testing.T, nc *nats.Conn) *heartbeat.Publisher {
	t.Helper()
	hb := heartbeat.New(testDaemonID, testUsername, "test-host",
		5*time.Minute, 30*time.Second, 30*time.Second,
		slog.Default())
	ready := make(chan error, 1)
	if err := hb.Start(t.Context(), nc, ready); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hb.Stop(t.Context()) })
	return hb
}

func startRegistry(
	t *testing.T,
	nc *nats.Conn,
	db *ledger.DB,
	hb *heartbeat.Publisher,
) *registry.Service {
	t.Helper()
	svc := registry.New(func() *ledger.DB { return db }, hb, nil,
		testUsername, testDaemonID,
		config.ReapingConfig{
			GracePeriod: config.Duration{Duration: 5 * time.Minute},
			Interval:    config.Duration{Duration: 30 * time.Second},
		},
		slog.Default())

	ready := make(chan error, 1)
	if err := svc.Start(t.Context(), nc, ready); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("registry start timeout")
	}
	t.Cleanup(func() { svc.Stop(t.Context()) })
	return svc
}

func publishLifecycle(
	t *testing.T,
	nc *nats.Conn,
	event *payload.FlowLifecycleEvent,
) {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	subject := broker.FlowLifecycleSubject(
		testUsername, event.FlowID)
	msgID := event.FlowID + "-" + string(event.Status)
	if _, err := js.Publish(subject, data,
		nats.MsgId(msgID)); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleProcessor_Register(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	event := &payload.FlowLifecycleEvent{
		FlowID:      "fl_TEST01",
		DaemonID:    testDaemonID,
		WorkspaceID: "ws_TESTWS01",
		Status:      payload.FlowActive,
		Metadata: map[string]string{
			"workspace_display_name": "myproject",
			"workspace_abs_path":     "/home/test/myproject",
		},
		Timestamp: time.Now().UTC(),
	}
	publishLifecycle(t, nc, event)

	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
		return len(flows) == 1
	}) {
		t.Fatal("expected 1 flow after lifecycle event")
	}

	flows, err := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if flows[0].FlowID != "fl_TEST01" {
		t.Errorf("flow_id = %q, want %q",
			flows[0].FlowID, "fl_TEST01")
	}
	if flows[0].DisplayName != "myproject" {
		t.Errorf("display_name = %q, want %q",
			flows[0].DisplayName, "myproject")
	}
	if flows[0].AbsPath != "/home/test/myproject" {
		t.Errorf("abs_path = %q, want %q",
			flows[0].AbsPath, "/home/test/myproject")
	}
}

func TestLifecycleProcessor_Terminate(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	// Register a flow.
	active := &payload.FlowLifecycleEvent{
		FlowID:      "fl_TEST02",
		DaemonID:    testDaemonID,
		WorkspaceID: "ws_TESTWS01",
		Status:      payload.FlowActive,
		Timestamp:   time.Now().UTC(),
	}
	publishLifecycle(t, nc, active)
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
		return len(flows) == 1
	}) {
		t.Fatal("expected 1 flow after active event")
	}

	// Terminate it.
	completed := &payload.FlowLifecycleEvent{
		FlowID:      "fl_TEST02",
		DaemonID:    testDaemonID,
		WorkspaceID: "ws_TESTWS01",
		Status:      payload.FlowCompleted,
		Timestamp:   time.Now().UTC(),
	}
	publishLifecycle(t, nc, completed)

	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
		return len(flows) == 0
	}) {
		t.Error("expected 0 flows after terminate")
	}
}

func TestStaleReaper(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)

	// Register a flow with an old timestamp directly in the DB.
	old := time.Now().UTC().Add(-10 * time.Minute)
	flow := &ledger.ActiveFlow{
		FlowID:                "fl_STALE01",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_TESTWS01",
		RegisteredAt:          old,
		LastActivityTimestamp: old,
	}
	if err := db.RegisterFlow(flow); err != nil {
		t.Fatal(err)
	}

	// Start registry with a very short grace period so the
	// reaper fires immediately on startup reconciliation.
	svc := registry.New(func() *ledger.DB { return db }, hb, nil,
		testUsername, testDaemonID,
		config.ReapingConfig{
			GracePeriod: config.Duration{Duration: 1 * time.Second},
			Interval:    config.Duration{Duration: 30 * time.Second},
		},
		slog.Default())

	ready := make(chan error, 1)
	if err := svc.Start(t.Context(), nc, ready); err != nil {
		t.Fatal(err)
	}
	<-ready
	t.Cleanup(func() { svc.Stop(t.Context()) })

	// The startup reconciliation should have reaped the flow.
	flows, err := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 0 {
		t.Errorf("got %d flows, want 0 (reaped)", len(flows))
	}
}

func TestFlowsEndpoint(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	// Register two flows.
	for _, id := range []string{"fl_EP01", "fl_EP02"} {
		event := &payload.FlowLifecycleEvent{
			FlowID:      id,
			DaemonID:    testDaemonID,
			WorkspaceID: "ws_TESTWS01",
			Status:      payload.FlowActive,
			Timestamp:   time.Now().UTC(),
		}
		publishLifecycle(t, nc, event)
	}
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
		return len(flows) == 2
	}) {
		t.Fatal("expected 2 flows")
	}

	// Query svc.flows.
	subject := broker.ServiceFlowsSubject(testUsername)
	resp, err := nc.Request(subject, []byte(`{}`), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.FlowsResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Flows) != 2 {
		t.Fatalf("got %d flows, want 2", len(result.Flows))
	}
	for _, f := range result.Flows {
		if f.FlowID == "" {
			t.Error("flow entry has empty flow_id")
		}
	}
}

func TestWorkspaceSnapshot(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	// Register flows in two different workspaces.
	events := []payload.FlowLifecycleEvent{
		{
			FlowID: "fl_WS01", DaemonID: testDaemonID,
			WorkspaceID: "ws_ALPHA",
			Status:      payload.FlowActive,
			Metadata: map[string]string{
				"workspace_display_name": "alpha",
				"workspace_abs_path":     "/home/test/alpha",
			},
			Timestamp: time.Now().UTC(),
		},
		{
			FlowID: "fl_WS02", DaemonID: testDaemonID,
			WorkspaceID: "ws_BETA",
			Status:      payload.FlowActive,
			Metadata: map[string]string{
				"workspace_display_name": "beta",
				"workspace_abs_path":     "/home/test/beta",
			},
			Timestamp: time.Now().UTC(),
		},
		{
			FlowID: "fl_WS03", DaemonID: testDaemonID,
			WorkspaceID: "ws_ALPHA",
			Status:      payload.FlowActive,
			Metadata: map[string]string{
				"workspace_display_name": "alpha",
				"workspace_abs_path":     "/home/test/alpha",
			},
			Timestamp: time.Now().UTC(),
		},
	}
	for i := range events {
		publishLifecycle(t, nc, &events[i])
	}
	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
		return len(flows) == 3
	}) {
		t.Fatal("expected 3 flows")
	}

	// Subscribe to heartbeat and trigger one.
	hbSubject := heartbeat.Subject(testUsername, testDaemonID)
	sub, err := nc.SubscribeSync(hbSubject)
	if err != nil {
		t.Fatal(err)
	}
	hb.Publish()

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var hbMsg heartbeat.DaemonHeartbeat
	if err := json.Unmarshal(msg.Data, &hbMsg); err != nil {
		t.Fatal(err)
	}

	if len(hbMsg.Workspaces) != 2 {
		t.Fatalf("got %d workspaces, want 2",
			len(hbMsg.Workspaces))
	}

	// Find alpha workspace and verify flow count.
	for _, ws := range hbMsg.Workspaces {
		switch ws.WorkspaceID {
		case "ws_ALPHA":
			if len(ws.ActiveFlows) != 2 {
				t.Errorf("alpha: got %d flows, want 2",
					len(ws.ActiveFlows))
			}
			if ws.DisplayName != "alpha" {
				t.Errorf("alpha display_name = %q, want %q",
					ws.DisplayName, "alpha")
			}
		case "ws_BETA":
			if len(ws.ActiveFlows) != 1 {
				t.Errorf("beta: got %d flows, want 1",
					len(ws.ActiveFlows))
			}
		default:
			t.Errorf("unexpected workspace: %s", ws.WorkspaceID)
		}
	}
}

// --- History endpoint tests (C-09) ---

func TestHistoryEndpoint_Empty(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	subject := broker.ServiceHistorySubject(testUsername)
	resp, err := nc.Request(subject, []byte(`{}`), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.HistoryQueryResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 0 {
		t.Errorf("got %d records, want 0", len(result.Records))
	}
	if result.Total != 0 {
		t.Errorf("total = %d, want 0", result.Total)
	}
}

func TestHistoryEndpoint_WithRecords(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	// Insert test records directly into the ledger.
	now := time.Now().UTC()
	wc := ledger.WriteContext{Username: testUsername}

	req1 := &payload.NotificationRequest{
		ID:            "ntf_HIST01",
		FlowID:        "fl_HIST01",
		DaemonID:      testDaemonID,
		WorkspaceID:   "ws_HISTWS",
		Title:         "First notification",
		Body:          "Body one",
		ResponseTypes: []payload.ResponseType{payload.ResponseBoolean},
		Priority:      payload.PriorityNormal,
		Source:        "test",
		Timestamp:     now.Add(-2 * time.Minute),
	}
	req2 := &payload.NotificationRequest{
		ID:            "ntf_HIST02",
		FlowID:        "fl_HIST01",
		DaemonID:      testDaemonID,
		WorkspaceID:   "ws_HISTWS",
		Title:         "Second notification",
		ResponseTypes: []payload.ResponseType{payload.ResponseChoice},
		Priority:      payload.PriorityHigh,
		Source:        "test",
		Actions:       []string{"Yes", "No"},
		Timestamp:     now.Add(-1 * time.Minute),
	}

	db.InsertRequest(wc, req1)
	db.InsertRequest(wc, req2)

	// Add a response for the first request.
	accepted := true
	resp1 := &payload.NotificationResponse{
		RequestID: "ntf_HIST01",
		Accepted:  &accepted,
		Timestamp: now.Add(-90 * time.Second),
	}
	db.InsertResponse(resp1)

	// Query all history.
	subject := broker.ServiceHistorySubject(testUsername)
	resp, err := nc.Request(subject, []byte(`{}`), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.HistoryQueryResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}

	if result.Total != 2 {
		t.Fatalf("total = %d, want 2", result.Total)
	}
	if len(result.Records) != 2 {
		t.Fatalf("got %d records, want 2", len(result.Records))
	}

	// Most recent first.
	if result.Records[0].Request.ID != "ntf_HIST02" {
		t.Errorf("records[0].id = %q, want ntf_HIST02",
			result.Records[0].Request.ID)
	}
	if result.Records[1].Request.ID != "ntf_HIST01" {
		t.Errorf("records[1].id = %q, want ntf_HIST01",
			result.Records[1].Request.ID)
	}

	// First record has a response.
	if result.Records[1].Response == nil {
		t.Error("records[1] should have a response")
	} else if result.Records[1].Response.Accepted == nil ||
		!*result.Records[1].Response.Accepted {
		t.Error("records[1].response should be accepted=true")
	}

	// Second record has no response.
	if result.Records[0].Response != nil {
		t.Error("records[0] should have no response")
	}
}

func TestHistoryEndpoint_WorkspaceFilter(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	now := time.Now().UTC()
	wc := ledger.WriteContext{Username: testUsername}

	for i, ws := range []string{"ws_A", "ws_A", "ws_B"} {
		req := &payload.NotificationRequest{
			ID:            fmt.Sprintf("ntf_WS%d", i),
			FlowID:        "fl_WS01",
			DaemonID:      testDaemonID,
			WorkspaceID:   ws,
			Title:         fmt.Sprintf("Notification %d", i),
			ResponseTypes: []payload.ResponseType{payload.ResponseNone},
			Priority:      payload.PriorityNormal,
			Timestamp:     now.Add(time.Duration(-i) * time.Minute),
		}
		db.InsertRequest(wc, req)
	}

	// Filter by workspace_id = ws_A.
	query := `{"workspace_id":"ws_A"}`
	subject := broker.ServiceHistorySubject(testUsername)
	resp, err := nc.Request(subject, []byte(query), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.HistoryQueryResult
	json.Unmarshal(resp.Data, &result)

	if result.Total != 2 {
		t.Errorf("total = %d, want 2 (ws_A only)", result.Total)
	}
	for _, rec := range result.Records {
		if rec.Request.WorkspaceID != "ws_A" {
			t.Errorf("got workspace %q, want ws_A",
				rec.Request.WorkspaceID)
		}
	}
}

func TestHistoryEndpoint_Pagination(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	now := time.Now().UTC()
	wc := ledger.WriteContext{Username: testUsername}

	for i := 0; i < 5; i++ {
		req := &payload.NotificationRequest{
			ID:            fmt.Sprintf("ntf_PAGE%02d", i),
			FlowID:        "fl_PAGE01",
			DaemonID:      testDaemonID,
			WorkspaceID:   "ws_PAGE",
			Title:         fmt.Sprintf("Page notification %d", i),
			ResponseTypes: []payload.ResponseType{payload.ResponseNone},
			Priority:      payload.PriorityNormal,
			Timestamp:     now.Add(time.Duration(-i) * time.Minute),
		}
		db.InsertRequest(wc, req)
	}

	// Page 1: limit=2, offset=0.
	query := `{"limit":2,"offset":0}`
	subject := broker.ServiceHistorySubject(testUsername)
	resp, err := nc.Request(subject, []byte(query), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.HistoryQueryResult
	json.Unmarshal(resp.Data, &result)

	if result.Total != 5 {
		t.Errorf("total = %d, want 5", result.Total)
	}
	if len(result.Records) != 2 {
		t.Errorf("page 1: got %d records, want 2",
			len(result.Records))
	}

	// Page 2: limit=2, offset=2.
	query = `{"limit":2,"offset":2}`
	resp, err = nc.Request(subject, []byte(query), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(resp.Data, &result)

	if len(result.Records) != 2 {
		t.Errorf("page 2: got %d records, want 2",
			len(result.Records))
	}

	// Page 3: limit=2, offset=4 — only 1 remaining.
	query = `{"limit":2,"offset":4}`
	resp, err = nc.Request(subject, []byte(query), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(resp.Data, &result)

	if len(result.Records) != 1 {
		t.Errorf("page 3: got %d records, want 1",
			len(result.Records))
	}
}

func TestStartupReconciliation(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)

	// Publish lifecycle events BEFORE starting the registry.
	event := &payload.FlowLifecycleEvent{
		FlowID:      "fl_RECON01",
		DaemonID:    testDaemonID,
		WorkspaceID: "ws_TESTWS01",
		Status:      payload.FlowActive,
		Metadata: map[string]string{
			"workspace_display_name": "reconproject",
			"workspace_abs_path":     "/home/test/recon",
		},
		Timestamp: time.Now().UTC(),
	}
	publishLifecycle(t, nc, event)

	// Brief pause to let NATS buffer the message before registry
	// starts. This is not a processing wait — it ensures the
	// message is in JetStream before the consumer binds.
	time.Sleep(50 * time.Millisecond)

	// Now start the registry — it should process the buffered
	// event via the lifecycle consumer.
	startRegistry(t, nc, db, hb)

	if !testutil.WaitFor(t, 2*time.Second, func() bool {
		flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
		return len(flows) == 1
	}) {
		t.Fatal("expected 1 flow after reconciliation")
	}

	flows, err := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if flows[0].FlowID != "fl_RECON01" {
	}
	if flows[0].FlowID != "fl_RECON01" {
		t.Errorf("flow_id = %q, want %q",
			flows[0].FlowID, "fl_RECON01")
	}
}

// --- Write endpoint tests (C-17, V-04) ---

func TestInsertRequestEndpoint(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	// Pre-register a flow so the request has context.
	flow := &ledger.ActiveFlow{
		FlowID:                "fl_WREQ01",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_WREQ01",
		DisplayName:           "wreqtest",
		AbsPath:               "/tmp/wreqtest",
		RegisteredAt:          time.Now().UTC(),
		LastActivityTimestamp: time.Now().UTC(),
	}
	db.RegisterFlow(flow)

	now := time.Now().UTC()
	cmd := statesvc.InsertRequestCmd{
		Username:      testUsername,
		FlowLabel:     "Test Flow",
		WorkspaceName: "wreqtest",
		WorkspacePath: "/tmp/wreqtest",
		Request: payload.NotificationRequest{
			ID:            "ntf_WREQ01",
			FlowID:        "fl_WREQ01",
			DaemonID:      testDaemonID,
			WorkspaceID:   "ws_WREQ01",
			Title:         "Write endpoint test",
			ResponseTypes: []payload.ResponseType{payload.ResponseNone},
			Priority:      payload.PriorityNormal,
			Timestamp:     now,
		},
	}
	data, _ := json.Marshal(cmd)

	subject := broker.ServiceInsertRequestSubject(testUsername)
	resp, err := nc.Request(subject, data, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.WriteResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Errorf("expected ok=true, got error: %s", result.Error)
	}

	// Verify the record is in the DB. The total includes both
	// lifecycle events (from RegisterFlow) and the notification.
	hist, err := db.QueryHistory(ledger.HistoryQuery{
		FlowID: "fl_WREQ01",
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Find the notification record among the results.
	var found bool
	for _, r := range hist.Records {
		if r.Type == ledger.HistoryTypeNotification &&
			r.Request != nil && r.Request.ID == "ntf_WREQ01" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("notification ntf_WREQ01 not found in %d history records",
			hist.Total)
	}
}

func TestInsertResponseEndpoint(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	// Insert a request first so the response can join.
	now := time.Now().UTC()
	db.InsertRequest(
		ledger.WriteContext{Username: testUsername},
		&payload.NotificationRequest{
			ID:            "ntf_WRSP01",
			FlowID:        "fl_WRSP01",
			DaemonID:      testDaemonID,
			WorkspaceID:   "ws_WRSP01",
			Title:         "Approve?",
			ResponseTypes: []payload.ResponseType{payload.ResponseBoolean},
			Priority:      payload.PriorityNormal,
			Timestamp:     now,
		})

	accepted := true
	cmd := statesvc.InsertResponseCmd{
		Response: payload.NotificationResponse{
			RequestID: "ntf_WRSP01",
			Accepted:  &accepted,
			Timestamp: now.Add(time.Second),
		},
	}
	data, _ := json.Marshal(cmd)

	subject := broker.ServiceInsertResponseSubject(testUsername)
	resp, err := nc.Request(subject, data, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.WriteResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Errorf("expected ok=true, got error: %s", result.Error)
	}

	// Verify the response joined with the request.
	hist, err := db.QueryHistory(ledger.HistoryQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rec := range hist.Records {
		if rec.Request != nil && rec.Request.ID == "ntf_WRSP01" &&
			rec.Response != nil {
			if rec.Response.Accepted == nil || !*rec.Response.Accepted {
				t.Error("expected accepted=true")
			}
			found = true
		}
	}
	if !found {
		t.Error("response not found in history")
	}
}

func TestInsertInterjectionEndpoint(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	now := time.Now().UTC()
	cmd := statesvc.InsertInterjectionCmd{
		Username: testUsername,
		Interjection: payload.InterjectionCommand{
			FlowID:    "fl_WINT01",
			Action:    payload.InterjectionStop,
			Context:   "Emergency stop",
			Timestamp: now,
		},
	}
	data, _ := json.Marshal(cmd)

	subject := broker.ServiceInsertInterjectionSubject(testUsername)
	resp, err := nc.Request(subject, data, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.WriteResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Errorf("expected ok=true, got error: %s", result.Error)
	}
}

func TestUpdateActivityEndpoint(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)
	startRegistry(t, nc, db, hb)

	// Register a flow.
	earlier := time.Now().UTC().Add(-5 * time.Minute)
	flow := &ledger.ActiveFlow{
		FlowID:                "fl_WACT01",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_WACT01",
		RegisteredAt:          earlier,
		LastActivityTimestamp: earlier,
	}
	db.RegisterFlow(flow)

	now := time.Now().UTC()
	cmd := statesvc.UpdateActivityCmd{
		FlowID:    "fl_WACT01",
		Timestamp: now,
	}
	data, _ := json.Marshal(cmd)

	subject := broker.ServiceUpdateActivitySubject(testUsername)
	resp, err := nc.Request(subject, data, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var result statesvc.WriteResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Errorf("expected ok=true, got error: %s", result.Error)
	}

	// Verify the timestamp was updated.
	flows, err := db.ListActiveFlows(ledger.ActiveFlowsQuery{
		FlowID: "fl_WACT01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("got %d flows, want 1", len(flows))
	}
	if !flows[0].LastActivityTimestamp.After(earlier) {
		t.Errorf("last_activity not updated: %v",
			flows[0].LastActivityTimestamp)
	}
}

func TestDevicePresenceEndpoint(t *testing.T) {
	srv, nc := startTestNATS(t)
	_ = srv
	db := openTestLedger(t)
	hb := newTestHeartbeat(t, nc)

	devices := []state.PairedDevice{
		{DeviceID: "mb_DEV01", PairedAt: time.Now().UTC()},
	}
	tracker := presence.New(testUsername, 5*time.Second,
		devices, slog.Default())
	trackerReady := make(chan error, 1)
	if err := tracker.Start(t.Context(), nc, trackerReady); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-trackerReady:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tracker start timeout")
	}
	t.Cleanup(func() { tracker.Stop(t.Context()) })

	// Start registry with the tracker.
	svc := registry.New(func() *ledger.DB { return db }, hb,
		tracker, testUsername, testDaemonID,
		config.ReapingConfig{
			GracePeriod: config.Duration{Duration: 5 * time.Minute},
			Interval:    config.Duration{Duration: 30 * time.Second},
		},
		slog.Default())
	ready := make(chan error, 1)
	if err := svc.Start(t.Context(), nc, ready); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("registry start timeout")
	}
	t.Cleanup(func() { svc.Stop(t.Context()) })

	// Query before any heartbeat — device should be offline.
	subject := broker.ServiceDevicePresenceSubject(testUsername)
	msg, err := nc.Request(subject, nil, 2*time.Second)
	if err != nil {
		t.Fatalf("presence query: %v", err)
	}
	var result statesvc.DevicePresenceResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(result.Devices))
	}
	if result.Devices[0].Online {
		t.Error("device should be offline before heartbeat")
	}

	// Publish a heartbeat.
	hbSubject := broker.DeviceHeartbeatSubject(testUsername,
		"mb_DEV01")
	hbData := []byte(`{"device_id":"mb_DEV01",` +
		`"timestamp":"2026-04-08T10:00:00Z"}`)
	if err := nc.Publish(hbSubject, hbData); err != nil {
		t.Fatal(err)
	}
	nc.Flush()
	time.Sleep(50 * time.Millisecond)

	// Query again — device should be online.
	msg, err = nc.Request(subject, nil, 2*time.Second)
	if err != nil {
		t.Fatalf("presence query after heartbeat: %v", err)
	}
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !result.Devices[0].Online {
		t.Error("device should be online after heartbeat")
	}
	if result.Devices[0].LastSeen == nil {
		t.Error("last_seen should be set")
	}
	if result.Devices[0].Username != testUsername {
		t.Errorf("username = %q, want %q",
			result.Devices[0].Username, testUsername)
	}
}
