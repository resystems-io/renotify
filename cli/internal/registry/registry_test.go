package registry_test

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/heartbeat"
	"go.resystems.io/renotify/internal/ledger"
	"go.resystems.io/renotify/internal/payload"
	"go.resystems.io/renotify/internal/registry"
	"go.resystems.io/renotify/internal/testutil"

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
		30*time.Second, slog.Default())
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
	svc := registry.New(func() *ledger.DB { return db }, hb, testUsername, testDaemonID,
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
	svc := registry.New(func() *ledger.DB { return db }, hb, testUsername, testDaemonID,
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

	var result registry.ActiveFlowsResult
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
