package heartbeat

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startTestServer creates an embedded NATS server and connected
// client for integration tests.
func startTestServer(t *testing.T) *nats.Conn {
	t.Helper()
	srv, err := broker.NewEmbeddedServer(broker.EmbeddedConfig{
		TCPHost:         "127.0.0.1",
		TCPPort:         -1,
		Username:        "testuser",
		InternalToken:   "testtoken",
		JetStreamMaxMem: 64 * 1024 * 1024,
	}, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Shutdown(context.Background()) })

	nc, err := broker.ConnectEmbedded(srv.Server(), "testtoken",
		discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nc.Close() })

	return nc
}

func TestSubject(t *testing.T) {
	got := Subject("stewart", "dn_3G2K7V9WNFQ4J")
	want := "resystems.renotify.stewart.daemon.dn_3G2K7V9WNFQ4J.heartbeat"
	if got != want {
		t.Errorf("Subject = %q, want %q", got, want)
	}
}

func TestPayload_JSON(t *testing.T) {
	hb := DaemonHeartbeat{
		DaemonID: "dn_TEST",
		Username: "testuser",
		Hostname: "dev-laptop",
		Workspaces: []WorkspaceInfo{
			{
				WorkspaceID: "ws_TEST",
				DisplayName: "myproject",
				AbsPath:     "/home/test/myproject",
				ActiveFlows: []FlowInfo{
					{FlowID: "fl_A", Label: "Build"},
					{FlowID: "fl_B", Label: "Test"},
				},
			},
		},
		Timestamp: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(hb)
	if err != nil {
		t.Fatal(err)
	}

	var decoded DaemonHeartbeat
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if decoded.DaemonID != "dn_TEST" {
		t.Errorf("daemon_id = %q", decoded.DaemonID)
	}
	if decoded.Username != "testuser" {
		t.Errorf("username = %q", decoded.Username)
	}
	if decoded.Hostname != "dev-laptop" {
		t.Errorf("hostname = %q", decoded.Hostname)
	}
	if len(decoded.Workspaces) != 1 {
		t.Fatalf("workspaces len = %d, want 1", len(decoded.Workspaces))
	}
	ws := decoded.Workspaces[0]
	if ws.WorkspaceID != "ws_TEST" {
		t.Errorf("workspace_id = %q", ws.WorkspaceID)
	}
	if len(ws.ActiveFlows) != 2 {
		t.Errorf("active_flows len = %d, want 2", len(ws.ActiveFlows))
	}
}

func TestPayload_EmptyWorkspaces(t *testing.T) {
	hb := DaemonHeartbeat{
		DaemonID:   "dn_TEST",
		Username:   "testuser",
		Hostname:   "dev-laptop",
		Workspaces: []WorkspaceInfo{},
		Timestamp:  time.Now().UTC(),
	}

	data, err := json.Marshal(hb)
	if err != nil {
		t.Fatal(err)
	}

	// Must serialise as [] not null.
	var raw map[string]json.RawMessage
	json.Unmarshal(data, &raw)
	if string(raw["workspaces"]) == "null" {
		t.Error("workspaces should be [] not null")
	}
	if string(raw["workspaces"]) != "[]" {
		t.Errorf("workspaces = %s, want []", raw["workspaces"])
	}
}

func TestPublisher_Name(t *testing.T) {
	p := New("dn_TEST", "testuser", "host",
		5*time.Minute, time.Second, discardLogger())
	if p.Name() != "heartbeat" {
		t.Errorf("Name = %q, want heartbeat", p.Name())
	}
}

func TestPublisher_ImmediatePublish(t *testing.T) {
	nc := startTestServer(t)

	subj := Subject("testuser", "dn_TEST")
	sub, err := nc.SubscribeSync(subj)
	if err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	p := New("dn_TEST", "testuser", "testhost",
		5*time.Minute, 10*time.Second, discardLogger())

	ready := make(chan error, 1)
	if err := p.Start(context.Background(), nc, ready); err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background())

	// Wait for the immediate heartbeat.
	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("expected immediate heartbeat: %v", err)
	}

	var hb DaemonHeartbeat
	if err := json.Unmarshal(msg.Data, &hb); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if hb.DaemonID != "dn_TEST" {
		t.Errorf("daemon_id = %q", hb.DaemonID)
	}
	if hb.Username != "testuser" {
		t.Errorf("username = %q", hb.Username)
	}
	if hb.Hostname != "testhost" {
		t.Errorf("hostname = %q", hb.Hostname)
	}
	if hb.Workspaces == nil {
		t.Error("workspaces should not be nil")
	}
	if hb.GracePeriod != "5m0s" {
		t.Errorf("grace_period = %q, want %q",
			hb.GracePeriod, "5m0s")
	}
}

func TestPublisher_PeriodicPublish(t *testing.T) {
	nc := startTestServer(t)

	subj := Subject("testuser", "dn_TEST")
	sub, err := nc.SubscribeSync(subj)
	if err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	// Use a short interval to keep the test fast.
	p := New("dn_TEST", "testuser", "testhost",
		5*time.Minute, 50*time.Millisecond, discardLogger())

	ready := make(chan error, 1)
	if err := p.Start(context.Background(), nc, ready); err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background())

	// Expect at least 3 heartbeats (1 immediate + 2 periodic)
	// within 200ms.
	for i := 0; i < 3; i++ {
		_, err := sub.NextMsg(time.Second)
		if err != nil {
			t.Fatalf("heartbeat %d: %v", i+1, err)
		}
	}
}

func TestPublisher_Stop(t *testing.T) {
	nc := startTestServer(t)

	subj := Subject("testuser", "dn_TEST")
	sub, err := nc.SubscribeSync(subj)
	if err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	p := New("dn_TEST", "testuser", "testhost",
		5*time.Minute, 50*time.Millisecond, discardLogger())

	ready := make(chan error, 1)
	if err := p.Start(context.Background(), nc, ready); err != nil {
		t.Fatal(err)
	}

	// Drain the immediate heartbeat.
	sub.NextMsg(time.Second)

	p.Stop(context.Background())

	// Wait long enough for a periodic tick to have fired.
	time.Sleep(150 * time.Millisecond)

	// No more heartbeats should arrive.
	_, err = sub.NextMsg(100 * time.Millisecond)
	if err != nats.ErrTimeout {
		t.Errorf("expected timeout after stop, got err=%v", err)
	}
}

func TestPublisher_SetWorkspaces(t *testing.T) {
	nc := startTestServer(t)

	subj := Subject("testuser", "dn_TEST")
	sub, err := nc.SubscribeSync(subj)
	if err != nil {
		t.Fatal(err)
	}
	nc.Flush()

	p := New("dn_TEST", "testuser", "testhost",
		5*time.Minute, 10*time.Second, discardLogger())

	ready := make(chan error, 1)
	if err := p.Start(context.Background(), nc, ready); err != nil {
		t.Fatal(err)
	}
	defer p.Stop(context.Background())

	// Drain the immediate heartbeat (empty workspaces).
	sub.NextMsg(time.Second)

	// Update workspaces and trigger an on-change heartbeat.
	p.SetWorkspaces([]WorkspaceInfo{
		{
			WorkspaceID: "ws_NEW",
			DisplayName: "new-project",
			AbsPath:     "/home/test/new-project",
			ActiveFlows: []FlowInfo{{FlowID: "fl_X"}},
		},
	})
	p.Publish()

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("expected on-change heartbeat: %v", err)
	}

	var hb DaemonHeartbeat
	json.Unmarshal(msg.Data, &hb)
	if len(hb.Workspaces) != 1 {
		t.Fatalf("workspaces len = %d, want 1", len(hb.Workspaces))
	}
	if hb.Workspaces[0].WorkspaceID != "ws_NEW" {
		t.Errorf("workspace_id = %q", hb.Workspaces[0].WorkspaceID)
	}
	if len(hb.Workspaces[0].ActiveFlows) != 1 {
		t.Errorf("active_flows len = %d, want 1",
			len(hb.Workspaces[0].ActiveFlows))
	}
}
