// Package registry implements the daemon's active flow registry
// service (C-10). It consumes FlowLifecycleEvent messages from
// the daemon-lifecycle JetStream consumer, maintains the SQLite
// active_flows table, reaps stale flows, serves the svc.flows
// Core NATS endpoint, and updates the heartbeat workspace
// snapshot.
package registry

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/heartbeat"
	"go.resystems.io/renotify/internal/ledger"
)

// Service is a daemon.Subsystem that manages the active flow
// registry. It depends on the ledger (for SQLite CRUD) and the
// heartbeat publisher (for workspace snapshot updates).
type Service struct {
	dbFunc   func() *ledger.DB
	hb       *heartbeat.Publisher
	username string
	daemonID string
	cfg      config.ReapingConfig
	logger   *slog.Logger

	nc     *nats.Conn
	sub    *nats.Subscription
	cancel context.CancelFunc
}

// New creates a registry Service. The dbFunc parameter is a lazy
// accessor that returns the ledger DB after the ledger subsystem
// has started.
func New(
	dbFunc func() *ledger.DB,
	hb *heartbeat.Publisher,
	username, daemonID string,
	cfg config.ReapingConfig,
	logger *slog.Logger,
) *Service {
	return &Service{
		dbFunc:   dbFunc,
		hb:       hb,
		username: username,
		daemonID: daemonID,
		cfg:      cfg,
		logger:   logger,
	}
}

// Name implements daemon.Subsystem.
func (s *Service) Name() string { return "registry" }

// Start implements daemon.Subsystem. It binds to the lifecycle
// JetStream consumer, starts the reaper goroutine, subscribes
// to the svc.flows endpoint, and runs initial reconciliation.
func (s *Service) Start(
	_ context.Context,
	nc *nats.Conn,
	ready chan<- error,
) error {
	s.nc = nc

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Start the lifecycle consumer goroutine.
	if err := s.startLifecycleConsumer(ctx); err != nil {
		cancel()
		if ready != nil {
			ready <- err
			close(ready)
		}
		return err
	}

	// Subscribe to svc.flows Core NATS endpoint.
	sub, err := s.subscribeFlowsEndpoint()
	if err != nil {
		cancel()
		if ready != nil {
			ready <- err
			close(ready)
		}
		return err
	}
	s.sub = sub

	// Start the stale flow reaper.
	go s.runReaper(ctx)

	// Initial reconciliation: reap stale flows that expired
	// while the daemon was down, then rebuild the workspace
	// snapshot. Buffered lifecycle events are processed by the
	// consumer goroutine which is already running.
	s.reapOnce()
	s.rebuildWorkspaceSnapshot()

	if ready != nil {
		close(ready)
	}
	return nil
}

// Stop implements daemon.Subsystem.
func (s *Service) Stop(_ context.Context) error {
	if s.cancel != nil {
		s.cancel()
	}
	if s.sub != nil {
		s.sub.Drain()
	}
	s.logger.Info("registry stopped")
	return nil
}

// reapOnce runs a single reaping pass. Used at startup and by
// the periodic reaper.
func (s *Service) reapOnce() {
	stale, err := s.dbFunc().ReapStaleFlows(s.cfg.GracePeriod.Duration)
	if err != nil {
		s.logger.Error("reap stale flows", "err", err)
		return
	}

	now := time.Now().UTC()
	for _, f := range stale {
		s.logger.Info("reaping stale flow",
			"flow_id", f.FlowID,
			"last_activity", f.LastActivityTimestamp)

		if err := s.dbFunc().TerminateFlow(
			f.FlowID, "failed", now); err != nil {
			s.logger.Error("terminate stale flow",
				"flow_id", f.FlowID, "err", err)
			continue
		}

		s.publishFailedLifecycle(f, now)
	}

	if len(stale) > 0 {
		s.rebuildWorkspaceSnapshot()
	}
}
