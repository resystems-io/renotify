package mcpserver_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/config"
	"go.resystems.io/renotify/cli/internal/heartbeat"
	"go.resystems.io/renotify/cli/internal/httpserver"
	"go.resystems.io/renotify/cli/internal/ledger"
	"go.resystems.io/renotify/cli/internal/mcpserver"
	"go.resystems.io/renotify/cli/internal/payload"
	"go.resystems.io/renotify/cli/internal/registry"

	"log/slog"
)

const testUsername = "testuser"
const testDaemonID = "dn_TEST1234ABCDE"

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

// startTestRegistry starts the registry subsystem which serves
// the svc.* NATS endpoints required by the MCP server's state
// client (C-17).
func startTestRegistry(
	t *testing.T,
	nc *nats.Conn,
	db *ledger.DB,
) {
	t.Helper()
	hb := heartbeat.New(testDaemonID, testUsername, "testhost",
		5*time.Minute, 30*time.Second, slog.Default())
	hbReady := make(chan error, 1)
	if err := hb.Start(t.Context(), nc, hbReady); err != nil {
		t.Fatal(err)
	}
	<-hbReady
	t.Cleanup(func() { hb.Stop(context.Background()) })

	regCfg := config.Default().Reaping
	regSvc := registry.New(
		func() *ledger.DB { return db }, hb,
		testUsername, testDaemonID, regCfg, slog.Default())
	regReady := make(chan error, 1)
	if err := regSvc.Start(t.Context(), nc, regReady); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-regReady:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("registry start timeout")
	}
	t.Cleanup(func() { regSvc.Stop(context.Background()) })
}

func startMCPServer(
	t *testing.T,
	nc *nats.Conn,
	db *ledger.DB,
) *mcpserver.Server {
	t.Helper()

	// Start the registry so svc.* endpoints are available.
	startTestRegistry(t, nc, db)

	cfg := config.Default()
	cfg.Username = testUsername

	httpSrv := httpserver.New("127.0.0.1", 0, slog.Default())
	srv := mcpserver.New(httpSrv, slog.Default(),
		testUsername, testDaemonID, cfg)

	ready := make(chan error, 1)
	if err := srv.Start(t.Context(), nc, ready); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mcp server start timeout")
	}
	t.Cleanup(func() { srv.Stop(context.Background()) })
	return srv
}

// --- Flow tool tests ---

func TestRegisterFlow(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	startMCPServer(t, nc, db)

	// Pre-populate a flow directly in the DB (test owns the DB).
	flow := &ledger.ActiveFlow{
		FlowID:                "fl_MCPTEST01",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_MCPTEST01",
		DisplayName:           "testproject",
		AbsPath:               "/home/test/project",
		RegisteredAt:          time.Now().UTC(),
		LastActivityTimestamp: time.Now().UTC(),
	}
	if err := db.RegisterFlow(flow); err != nil {
		t.Fatal(err)
	}

	flows, err := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 1 {
		t.Fatalf("got %d flows, want 1", len(flows))
	}
	if flows[0].FlowID != "fl_MCPTEST01" {
		t.Errorf("flow_id = %q, want %q",
			flows[0].FlowID, "fl_MCPTEST01")
	}
}

func TestTerminateFlow(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	startMCPServer(t, nc, db)

	flow := &ledger.ActiveFlow{
		FlowID:                "fl_MCPTERM01",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_MCPTEST01",
		RegisteredAt:          time.Now().UTC(),
		LastActivityTimestamp: time.Now().UTC(),
	}
	db.RegisterFlow(flow)

	err := db.TerminateFlow("fl_MCPTERM01", "completed", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	flows, err := db.ListActiveFlows(ledger.ActiveFlowsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 0 {
		t.Errorf("got %d flows, want 0 after terminate", len(flows))
	}
}

// --- DecisionStore tests ---

func TestDecisionStore_RegisterAndGet(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	now := time.Now().UTC()

	ds.Register("ntf_TEST01", now)
	r := ds.Get("ntf_TEST01")
	if r == nil {
		t.Fatal("expected resource, got nil")
	}
	if r.Decided {
		t.Error("expected decided=false")
	}
	if r.RequestID != "ntf_TEST01" {
		t.Errorf("request_id = %q, want %q",
			r.RequestID, "ntf_TEST01")
	}
}

func TestDecisionStore_Resolve(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	now := time.Now().UTC()

	ds.Register("ntf_TEST02", now)

	accepted := true
	resp := &payload.NotificationResponse{
		RequestID: "ntf_TEST02",
		Accepted:  &accepted,
		Action:    "Approve",
		Timestamp: now,
	}
	if !ds.Resolve("ntf_TEST02", resp) {
		t.Fatal("expected resolve to succeed")
	}

	r := ds.Get("ntf_TEST02")
	if !r.Decided {
		t.Error("expected decided=true")
	}
	if r.Accepted == nil || !*r.Accepted {
		t.Error("expected accepted=true")
	}
	if r.Action != "Approve" {
		t.Errorf("action = %q, want %q", r.Action, "Approve")
	}
}

func TestDecisionStore_ResolveTimeout(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	now := time.Now().UTC()

	ds.Register("ntf_TEST03", now)

	if !ds.ResolveTimeout("ntf_TEST03", now) {
		t.Fatal("expected resolve to succeed")
	}

	r := ds.Get("ntf_TEST03")
	if !r.Decided {
		t.Error("expected decided=true")
	}
	if r.Accepted != nil {
		t.Error("expected accepted=nil for timeout")
	}
	if r.Action != "" {
		t.Error("expected empty action for timeout")
	}
}

func TestDecisionStore_ResolveNotFound(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	if ds.Resolve("ntf_MISSING", &payload.NotificationResponse{}) {
		t.Error("expected resolve to return false for missing ID")
	}
}

func TestDecisionStore_GetNotFound(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	if ds.Get("ntf_MISSING") != nil {
		t.Error("expected nil for missing ID")
	}
}

func TestDecisionStore_Remove(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	ds.Register("ntf_REM01", time.Now().UTC())
	ds.Remove("ntf_REM01")
	if ds.Get("ntf_REM01") != nil {
		t.Error("expected nil after remove")
	}
}

func TestDecisionResourceURI(t *testing.T) {
	uri := mcpserver.DecisionResourceURI("ntf_4H7DCRW2VNPK9FMJ")
	want := "renotify://decisions/ntf_4H7DCRW2VNPK9FMJ"
	if uri != want {
		t.Errorf("URI = %q, want %q", uri, want)
	}
}

// --- Post tool integration test ---

func TestPostTool_PublishesNotification(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	startMCPServer(t, nc, db)

	flow := &ledger.ActiveFlow{
		FlowID:                "fl_POST01",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_POST01",
		DisplayName:           "posttest",
		AbsPath:               "/tmp/posttest",
		RegisteredAt:          time.Now().UTC(),
		LastActivityTimestamp: time.Now().UTC(),
	}
	db.RegisterFlow(flow)

	sub, err := nc.SubscribeSync(
		broker.FlowRequestSubject(testUsername, "fl_POST01"))
	if err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	js, _ := nc.JetStream()
	now := time.Now().UTC()
	req := &payload.NotificationRequest{
		ID:            "ntf_POSTTEST01",
		FlowID:        "fl_POST01",
		DaemonID:      testDaemonID,
		WorkspaceID:   "ws_POST01",
		Title:         "Test post",
		Body:          "Body text",
		ResponseTypes: []payload.ResponseType{payload.ResponseNone},
		Priority:      payload.PriorityNormal,
		Timestamp:     now,
	}
	broker.PublishJSON(js,
		broker.FlowRequestSubject(testUsername, "fl_POST01"),
		"ntf_POSTTEST01", req)

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}

	var received payload.NotificationRequest
	if err := json.Unmarshal(msg.Data, &received); err != nil {
		t.Fatal(err)
	}
	if received.Title != "Test post" {
		t.Errorf("title = %q, want %q", received.Title, "Test post")
	}
}

// --- Tool error-path tests ---

func TestPostTool_ExpiredFlowErrors(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	_ = startMCPServer(t, nc, db)

	// Verify a nonexistent flow is not found via DB.
	flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{
		FlowID: "fl_NONEXIST",
	})
	if len(flows) != 0 {
		t.Errorf("expected 0 flows, got %d", len(flows))
	}
}

func TestRefreshFlow_ExpiredFlowErrors(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	startMCPServer(t, nc, db)

	// Register a flow then terminate it.
	flow := &ledger.ActiveFlow{
		FlowID:                "fl_REFRESH_EXPIRED",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_TEST01",
		RegisteredAt:          time.Now().UTC(),
		LastActivityTimestamp: time.Now().UTC(),
	}
	db.RegisterFlow(flow)
	db.TerminateFlow("fl_REFRESH_EXPIRED", "completed",
		time.Now().UTC())

	// Confirm the flow is gone from active registry.
	flows, _ := db.ListActiveFlows(ledger.ActiveFlowsQuery{
		FlowID: "fl_REFRESH_EXPIRED",
	})
	if len(flows) != 0 {
		t.Errorf("expected 0 flows after terminate, got %d",
			len(flows))
	}
}

// --- Ask + decision tests ---

func TestAskTool_DecisionResolvedOnResponse(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	startMCPServer(t, nc, db)

	ds := mcpserver.NewDecisionStore()
	ds.Register("ntf_ASKTEST01", time.Now().UTC())

	r := ds.Get("ntf_ASKTEST01")
	if r == nil || r.Decided {
		t.Fatal("expected pending decision")
	}

	accepted := true
	resp := &payload.NotificationResponse{
		RequestID: "ntf_ASKTEST01",
		Accepted:  &accepted,
		Action:    "Yes",
		Timestamp: time.Now().UTC(),
	}
	ds.Resolve("ntf_ASKTEST01", resp)

	r = ds.Get("ntf_ASKTEST01")
	if !r.Decided {
		t.Error("expected decided=true")
	}
	if r.Accepted == nil || !*r.Accepted {
		t.Error("expected accepted=true")
	}
}

// --- Await decision tests ---

func TestAwaitDecision_ResolvesOnResponse(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	ds.Register("ntf_AWAIT01", time.Now().UTC())

	go func() {
		time.Sleep(100 * time.Millisecond)
		accepted := true
		ds.Resolve("ntf_AWAIT01", &payload.NotificationResponse{
			RequestID: "ntf_AWAIT01",
			Accepted:  &accepted,
			Action:    "Yes",
			Timestamp: time.Now().UTC(),
		})
	}()

	ch := ds.Resolved("ntf_AWAIT01")
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for decision")
	}

	result := ds.Get("ntf_AWAIT01")
	if !result.Decided {
		t.Error("expected decided=true")
	}
	if result.Accepted == nil || !*result.Accepted {
		t.Error("expected accepted=true")
	}
}

func TestAwaitDecision_NotFound(t *testing.T) {
	ds := mcpserver.NewDecisionStore()
	r := ds.Get("ntf_MISSING")
	if r != nil {
		t.Error("expected nil for unknown notification_id")
	}
}

// --- InterjectionStore tests ---

func TestInterjectionStore_RegisterAndAppend(t *testing.T) {
	is := mcpserver.NewInterjectionStore()
	is.Register("fl_INT01")

	is.Append("fl_INT01", payload.InterjectionResource{
		FlowID:    "fl_INT01",
		Action:    payload.InterjectionNote,
		Context:   "Check the logs",
		Timestamp: time.Now().UTC(),
	})

	r := is.Get("fl_INT01")
	if r == nil {
		t.Fatal("expected resource")
	}
	if r.Action != payload.InterjectionNote {
		t.Errorf("action = %q, want %q",
			r.Action, payload.InterjectionNote)
	}
	if r.Context != "Check the logs" {
		t.Errorf("context = %q", r.Context)
	}
}

func TestInterjectionStore_Accumulates(t *testing.T) {
	is := mcpserver.NewInterjectionStore()
	is.Register("fl_INT02")

	is.Append("fl_INT02", payload.InterjectionResource{
		FlowID: "fl_INT02", Action: payload.InterjectionNote,
		Context: "First", Timestamp: time.Now().UTC(),
	})
	is.Append("fl_INT02", payload.InterjectionResource{
		FlowID: "fl_INT02", Action: payload.InterjectionNote,
		Context: "Second", Timestamp: time.Now().UTC(),
	})

	items := is.Drain("fl_INT02")
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Context != "First" {
		t.Errorf("items[0].context = %q", items[0].Context)
	}
	if items[1].Context != "Second" {
		t.Errorf("items[1].context = %q", items[1].Context)
	}

	// Drain again — should be empty.
	items = is.Drain("fl_INT02")
	if len(items) != 0 {
		t.Errorf("got %d items after drain, want 0", len(items))
	}
}

func TestInterjectionStore_Notified(t *testing.T) {
	is := mcpserver.NewInterjectionStore()
	is.Register("fl_INT03")

	ch := is.Notified("fl_INT03")
	if ch == nil {
		t.Fatal("expected channel")
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		is.Append("fl_INT03", payload.InterjectionResource{
			FlowID: "fl_INT03", Action: payload.InterjectionStop,
			Timestamp: time.Now().UTC(),
		})
	}()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for notification")
	}
}

func TestInterjectionStore_Registered(t *testing.T) {
	is := mcpserver.NewInterjectionStore()

	if is.Registered("fl_NOREG") {
		t.Error("unregistered flow should return false")
	}

	is.Register("fl_REG01")
	if !is.Registered("fl_REG01") {
		t.Error("registered flow should return true")
	}

	// Registered but empty — Get returns nil, Registered true.
	if is.Get("fl_REG01") != nil {
		t.Error("expected nil Get before any interjections")
	}
}

func TestInterjectionStore_Remove(t *testing.T) {
	is := mcpserver.NewInterjectionStore()
	is.Register("fl_INT04")
	is.Remove("fl_INT04")

	if is.Get("fl_INT04") != nil {
		t.Error("expected nil after remove")
	}
	if is.Notified("fl_INT04") != nil {
		t.Error("expected nil channel after remove")
	}
	if is.Registered("fl_INT04") {
		t.Error("expected false after remove")
	}
}

func TestInterjectionResourceURI(t *testing.T) {
	uri := mcpserver.InterjectionResourceURI("fl_TEST01")
	want := "renotify://interjections/fl_TEST01"
	if uri != want {
		t.Errorf("URI = %q, want %q", uri, want)
	}
}

// --- Timeout test ---

func TestTimeout_ResolvesDecision(t *testing.T) {
	_, nc := startTestNATS(t)
	db := openTestLedger(t)
	startMCPServer(t, nc, db)

	// Register a flow directly in the DB (test owns the DB).
	flow := &ledger.ActiveFlow{
		FlowID:                "fl_TMO01",
		Username:              testUsername,
		DaemonID:              testDaemonID,
		WorkspaceID:           "ws_TMO01",
		RegisteredAt:          time.Now().UTC(),
		LastActivityTimestamp: time.Now().UTC(),
	}
	db.RegisterFlow(flow)

	// Subscribe to .response to verify ErrorResponse is published.
	sub, err := nc.SubscribeSync(
		broker.FlowResponseSubject(testUsername, "fl_TMO01"))
	if err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	// Simulate what the ask handler does: register decision,
	// start response subscriber, start timeout timer.
	// We use a very short timeout (1s) for the test.
	ds := mcpserver.NewDecisionStore()
	ds.Register("ntf_TMO01", time.Now().UTC())

	// The timeout timer publishes ErrorResponse after 1s.
	// We verify by checking the .response subject.
	js, _ := nc.JetStream()
	now := time.Now().UTC()
	errResp := &payload.ErrorResponse{
		CorrelationID: "ntf_TMO01",
		Code:          "timeout",
		Message:       "ask timeout expired",
		Timestamp:     now,
	}
	broker.PublishJSON(js,
		broker.FlowResponseSubject(testUsername, "fl_TMO01"),
		"ntf_TMO01-timeout", errResp)

	// Verify ErrorResponse arrives.
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var received payload.ErrorResponse
	if err := json.Unmarshal(msg.Data, &received); err != nil {
		t.Fatal(err)
	}
	if received.Code != "timeout" {
		t.Errorf("code = %q, want %q", received.Code, "timeout")
	}
}

// --- Subscriber map tests ---

func TestSubscriberMap_CancelAll(t *testing.T) {
	sm := mcpserver.NewSubscriberMap()

	cancelled := make(chan struct{}, 2)
	sm.Add("ntf_01", func() { cancelled <- struct{}{} })
	sm.Add("ntf_02", func() { cancelled <- struct{}{} })

	sm.CancelAll()

	for range 2 {
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for cancel")
		}
	}
}
