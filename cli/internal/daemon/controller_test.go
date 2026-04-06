package daemon

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testConfig() *config.Config {
	cfg := config.Default()
	cfg.Username = "testuser"
	cfg.Broker.TCPPort = -1 // NATS convention for random port
	cfg.Broker.WSSPort = -1
	cfg.Broker.CertFile = "" // skip WSS in tests
	cfg.Broker.KeyFile = ""
	cfg.MCP.Port = 0
	return cfg
}

// waitReady blocks until the controller signals all subsystems
// are ready, or fails after 5 seconds.
func waitReady(t *testing.T, c *Controller) {
	t.Helper()
	select {
	case <-c.Ready:
	case <-time.After(5 * time.Second):
		t.Fatal("daemon startup timeout")
	}
}

func TestNewController_Defaults(t *testing.T) {
	cfg := testConfig()
	c := NewController(cfg)
	if c.cfg != cfg {
		t.Error("config not stored")
	}
	if len(c.subsystems) != 0 {
		t.Error("expected no subsystems by default")
	}
}

func TestNewController_WithSubsystem(t *testing.T) {
	cfg := testConfig()
	mock := &mockSubsystem{name: "test"}
	c := NewController(cfg, WithSubsystem(mock))
	if len(c.subsystems) != 1 {
		t.Fatalf("expected 1 subsystem, got %d", len(c.subsystems))
	}
	if c.subsystems[0].Name() != "test" {
		t.Errorf("name = %q, want %q", c.subsystems[0].Name(), "test")
	}
}

func TestController_ShutdownOnCancel(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()
	c := NewController(cfg, WithLogger(testLogger()))
	c.DaemonIDPath = dir + "/daemon_id"
	c.InternalTokenPath = dir + "/internal_token"
	c.PairingTokenPath = dir + "/pairing/token"

	ctx, cancel := context.WithCancel(context.Background())

	c.Ready = make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- c.Run(ctx)
	}()

	waitReady(t, c)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil on clean shutdown, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for shutdown")
	}
}

func TestController_WaitsForReady(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()

	readyCalled := make(chan struct{})
	mock := &mockSubsystem{
		name: "slow",
		startFn: func(ctx context.Context, nc *nats.Conn, ready chan<- error) error {
			go func() {
				time.Sleep(100 * time.Millisecond)
				close(ready)
				close(readyCalled)
			}()
			return nil
		},
	}

	c := NewController(cfg, WithLogger(testLogger()), WithSubsystem(mock))
	c.DaemonIDPath = dir + "/daemon_id"
	c.InternalTokenPath = dir + "/internal_token"
	c.PairingTokenPath = dir + "/pairing/token"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-readyCalled
		cancel()
	}()

	err := c.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestController_ReadyError(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()

	mock := &mockSubsystem{
		name: "failing",
		startFn: func(ctx context.Context, nc *nats.Conn, ready chan<- error) error {
			go func() {
				ready <- fmt.Errorf("init failed")
				close(ready)
			}()
			return nil
		},
	}

	c := NewController(cfg, WithLogger(testLogger()), WithSubsystem(mock))
	c.DaemonIDPath = dir + "/daemon_id"
	c.InternalTokenPath = dir + "/internal_token"
	c.PairingTokenPath = dir + "/pairing/token"

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.Run(ctx)
	if err == nil {
		t.Fatal("expected error from failing subsystem")
	}
	if err.Error() != "subsystem failing: init failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestController_StartupTimeout(t *testing.T) {
	cfg := testConfig()
	dir := t.TempDir()

	mock := &mockSubsystem{
		name: "stuck",
		startFn: func(ctx context.Context, nc *nats.Conn, ready chan<- error) error {
			// Never signals ready — rely on controller timeout.
			// But we still must close the channel eventually to
			// avoid a leak, so do it on context cancellation.
			go func() {
				<-ctx.Done()
				close(ready)
			}()
			return nil
		},
	}

	c := NewController(cfg,
		WithLogger(testLogger()),
		WithSubsystem(mock),
		WithStartupTimeout(200*time.Millisecond),
	)
	c.DaemonIDPath = dir + "/daemon_id"
	c.InternalTokenPath = dir + "/internal_token"
	c.PairingTokenPath = dir + "/pairing/token"

	// Use a context that outlives the startup timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := c.Run(ctx)
	if err == nil {
		t.Fatal("expected startup timeout error")
	}
	if err.Error() != "subsystem stuck: startup timeout" {
		t.Errorf("unexpected error: %v", err)
	}
}

// mockSubsystem is a test double for Subsystem.
type mockSubsystem struct {
	name    string
	startFn func(ctx context.Context, nc *nats.Conn, ready chan<- error) error
	stopFn  func()
	stopped bool
}

func (m *mockSubsystem) Name() string { return m.name }

func (m *mockSubsystem) Start(ctx context.Context, nc *nats.Conn, ready chan<- error) error {
	if m.startFn != nil {
		return m.startFn(ctx, nc, ready)
	}
	if ready != nil {
		close(ready)
	}
	return nil
}

func (m *mockSubsystem) Stop(_ context.Context) error {
	m.stopped = true
	if m.stopFn != nil {
		m.stopFn()
	}
	return nil
}
