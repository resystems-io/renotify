// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package heartbeat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// Publisher is a daemon.Subsystem that publishes periodic
// DaemonHeartbeat messages over Core NATS Pub/Sub. It publishes
// an immediate heartbeat on Start and then every interval.
//
// The workspaces snapshot is empty until populated by
// SetWorkspaces (called by the flow registry in later phases).
// Publish triggers an immediate out-of-cycle heartbeat for
// on-change events (flow started/ended, workspace added/removed).
type Publisher struct {
	daemonID    string
	username    string
	hostname    string
	gracePeriod time.Duration
	interval    time.Duration
	logger      *slog.Logger

	nc     *nats.Conn
	cancel context.CancelFunc

	mu         sync.RWMutex
	workspaces []WorkspaceInfo
}

// New creates a heartbeat Publisher. Call Start to begin
// publishing. The gracePeriod is included in each heartbeat
// so the mobile app can compute flow TTL locally.
func New(daemonID, username, hostname string,
	gracePeriod, interval time.Duration,
	logger *slog.Logger) *Publisher {
	return &Publisher{
		daemonID:    daemonID,
		username:    username,
		hostname:    hostname,
		gracePeriod: gracePeriod,
		interval:    interval,
		logger:      logger,
		workspaces:  []WorkspaceInfo{},
	}
}

// Name implements daemon.Subsystem.
func (p *Publisher) Name() string { return "heartbeat" }

// Start implements daemon.Subsystem. It publishes an immediate
// heartbeat, starts the periodic ticker, and signals ready.
func (p *Publisher) Start(_ context.Context, nc *nats.Conn, ready chan<- error) error {
	p.nc = nc

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	// Publish the first heartbeat immediately (Section 8.1
	// step 12 / Section 8.2 step 8).
	p.publish()

	go p.run(ctx)

	close(ready)
	return nil
}

// Stop implements daemon.Subsystem. It cancels the periodic
// ticker goroutine.
func (p *Publisher) Stop(_ context.Context) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

// Publish triggers an immediate out-of-cycle heartbeat. Use
// this for on-change events (flow started/ended, workspace
// added/removed). Thread-safe.
func (p *Publisher) Publish() {
	p.publish()
}

// SetWorkspaces replaces the workspace snapshot. The updated
// snapshot is included in the next heartbeat (periodic or
// on-change). Thread-safe.
func (p *Publisher) SetWorkspaces(ws []WorkspaceInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.workspaces = ws
}

// run is the periodic ticker goroutine.
func (p *Publisher) run(ctx context.Context) {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.publish()
		}
	}
}

// publish marshals and publishes a single heartbeat.
func (p *Publisher) publish() {
	p.mu.RLock()
	ws := make([]WorkspaceInfo, len(p.workspaces))
	copy(ws, p.workspaces)
	p.mu.RUnlock()

	hb := DaemonHeartbeat{
		DaemonID:    p.daemonID,
		Username:    p.username,
		Hostname:    p.hostname,
		GracePeriod: p.gracePeriod.String(),
		Workspaces:  ws,
		Timestamp:   time.Now().UTC(),
	}

	data, err := json.Marshal(hb)
	if err != nil {
		p.logger.Error("heartbeat marshal", "err", err)
		return
	}

	subject := Subject(p.username, p.daemonID)
	if err := p.nc.Publish(subject, data); err != nil {
		p.logger.Error("heartbeat publish", "err", err,
			"subject", subject)
		return
	}

	p.logger.Debug("heartbeat published", "subject", subject)
}
