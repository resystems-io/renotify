package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/state"
	"go.resystems.io/renotify/internal/xdg"
)

const defaultStartupTimeout = 10 * time.Second

// Controller orchestrates the daemon lifecycle: state loading,
// broker startup (or shared connection), subsystem startup, and
// graceful shutdown.
type Controller struct {
	cfg            *config.Config
	logger         *slog.Logger
	subsystems     []Subsystem
	startupTimeout time.Duration

	// ReloadCh receives a signal (e.g., from SIGHUP) to reload
	// the pairing token and update the embedded broker's auth.
	// Nil disables reload support.
	ReloadCh <-chan os.Signal

	// Overridable paths for testing.
	DaemonIDPath      string
	InternalTokenPath string
	PairingTokenPath  string
}

// Option configures a Controller.
type Option func(*Controller)

// WithSubsystem registers a Subsystem to start after the NATS
// client connection is established.
func WithSubsystem(s Subsystem) Option {
	return func(c *Controller) {
		c.subsystems = append(c.subsystems, s)
	}
}

// WithStartupTimeout overrides the per-subsystem startup timeout.
func WithStartupTimeout(d time.Duration) Option {
	return func(c *Controller) {
		c.startupTimeout = d
	}
}

// WithLogger overrides the default logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Controller) {
		c.logger = l
	}
}

// NewController creates a Controller from the validated config.
func NewController(cfg *config.Config, opts ...Option) *Controller {
	c := &Controller{
		cfg:               cfg,
		logger:            slog.Default(),
		startupTimeout:    defaultStartupTimeout,
		DaemonIDPath:      xdg.DaemonIDPath(),
		InternalTokenPath: xdg.InternalTokenPath(),
		PairingTokenPath:  xdg.PairingTokenPath(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Run executes the daemon lifecycle. It blocks until ctx is
// cancelled or a fatal error occurs. Returns nil on clean
// shutdown.
func (c *Controller) Run(ctx context.Context) error {
	// 1. Load daemon_id.
	daemonID, err := state.LoadOrGenerateDaemonID(c.DaemonIDPath)
	if err != nil {
		return fmt.Errorf("daemon_id: %w", err)
	}
	c.logger.Info("daemon identity loaded", "daemon_id", daemonID)

	// 2. Branch on embedded vs shared broker.
	var nc *nats.Conn
	var embeddedSrv *broker.EmbeddedServer

	if c.cfg.Broker.Enabled {
		nc, embeddedSrv, err = c.startEmbedded(ctx)
	} else {
		nc, err = c.startShared()
	}
	if err != nil {
		return err
	}
	defer func() {
		if nc != nil {
			nc.Drain()
		}
		if embeddedSrv != nil {
			embeddedSrv.Shutdown(context.Background())
		}
	}()

	// 2.5. Ensure JetStream stream and consumers.
	if err := broker.EnsureJetStream(ctx, nc,
		c.cfg.Username, c.cfg.JetStream, c.logger); err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}

	// 3. Start subsystems with ready-wait.
	started := make([]Subsystem, 0, len(c.subsystems))
	defer func() {
		// Stop in reverse order.
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for i := len(started) - 1; i >= 0; i-- {
			if err := started[i].Stop(shutCtx); err != nil {
				c.logger.Error("subsystem stop error",
					"name", started[i].Name(), "err", err)
			}
		}
	}()

	for _, sub := range c.subsystems {
		ready := make(chan error, 1)
		if err := sub.Start(ctx, nc, ready); err != nil {
			return fmt.Errorf("subsystem %s: %w", sub.Name(), err)
		}
		select {
		case err := <-ready:
			if err != nil {
				return fmt.Errorf("subsystem %s: %w", sub.Name(), err)
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.startupTimeout):
			return fmt.Errorf("subsystem %s: startup timeout", sub.Name())
		}
		started = append(started, sub)
		c.logger.Info("subsystem ready", "name", sub.Name())
	}

	mode := "embedded"
	if !c.cfg.Broker.Enabled {
		mode = "shared"
	}
	c.logger.Info("daemon started",
		"daemon_id", daemonID,
		"mode", mode,
		"username", c.cfg.Username,
	)

	// 4. Block until context cancelled, handling reload signals.
	for {
		if c.ReloadCh == nil {
			<-ctx.Done()
			break
		}
		select {
		case <-ctx.Done():
			break
		case <-c.ReloadCh:
			c.reloadAuth(embeddedSrv)
			continue
		}
		break
	}
	c.logger.Info("shutting down")
	return nil
}

// reloadAuth re-reads the pairing token from disk and applies
// the updated auth configuration to the embedded broker.
func (c *Controller) reloadAuth(srv *broker.EmbeddedServer) {
	c.logger.Info("reloading auth configuration (SIGHUP)")

	if srv == nil {
		c.logger.Warn("auth reload skipped: shared broker mode")
		return
	}

	// Re-read tokens from disk.
	internalToken, err := state.LoadOrGenerateToken(c.InternalTokenPath)
	if err != nil {
		c.logger.Error("reload: read internal token", "err", err)
		return
	}
	pairingToken, err := state.LoadPairingToken(c.PairingTokenPath)
	if err != nil {
		c.logger.Error("reload: read pairing token", "err", err)
		return
	}

	if err := srv.ReloadAuth(c.cfg.Username, internalToken, pairingToken); err != nil {
		c.logger.Error("reload: apply auth config", "err", err)
		return
	}

	if pairingToken != "" {
		c.logger.Info("auth reloaded",
			"pairing_token", pairingToken[:10]+"...")
	} else {
		c.logger.Info("auth reloaded", "pairing_token", "(none)")
	}
}

func (c *Controller) startEmbedded(_ context.Context) (*nats.Conn, *broker.EmbeddedServer, error) {
	// Load TLS (optional — skip WSS if missing).
	cert, key, err := state.LoadTLS(
		c.cfg.Broker.CertFile, c.cfg.Broker.KeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("TLS: %w", err)
	}
	if cert == "" {
		c.logger.Warn("TLS cert/key not found, WSS listener disabled")
	}

	// Load internal token.
	internalToken, err := state.LoadOrGenerateToken(c.InternalTokenPath)
	if err != nil {
		return nil, nil, fmt.Errorf("internal token: %w", err)
	}

	// Load pairing token (optional).
	pairingToken, err := state.LoadPairingToken(c.PairingTokenPath)
	if err != nil {
		return nil, nil, fmt.Errorf("pairing token: %w", err)
	}

	// Configure and start embedded NATS.
	srv, err := broker.NewEmbeddedServer(broker.EmbeddedConfig{
		TCPHost:         c.cfg.Broker.TCPHost,
		TCPPort:         c.cfg.Broker.TCPPort,
		WSSHost:         c.cfg.Broker.WSSHost,
		WSSPort:         c.cfg.Broker.WSSPort,
		TLSCert:         cert,
		TLSKey:          key,
		Username:        c.cfg.Username,
		InternalToken:   internalToken,
		PairingToken:    pairingToken,
		JetStreamMaxMem: c.cfg.JetStream.MaxBytes,
	}, c.logger)
	if err != nil {
		return nil, nil, err
	}
	if err := srv.Start(); err != nil {
		return nil, nil, err
	}

	// Connect as NATS client.
	nc, err := broker.ConnectEmbedded(srv.ClientURL(), internalToken, c.logger)
	if err != nil {
		srv.Shutdown(context.Background())
		return nil, nil, err
	}

	return nc, srv, nil
}

func (c *Controller) startShared() (*nats.Conn, error) {
	nc, err := broker.ConnectShared(c.cfg.SharedBroker, c.logger)
	if err != nil {
		return nil, err
	}
	return nc, nil
}
