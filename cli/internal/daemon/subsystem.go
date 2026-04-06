// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package daemon implements the daemon controller that
// orchestrates the NATS broker, HTTP server, MCP server, and
// future subsystems.
package daemon

import (
	"context"

	"github.com/nats-io/nats.go"
)

// Subsystem is the interface that daemon components implement to
// participate in the daemon lifecycle. See the C-02 plan DD-4 for
// the ready-signalling rationale.
//
// The subsystem always closes the ready channel:
//   - Success: close(ready)
//   - Failure: ready <- err; close(ready)
//
// If ready is nil the subsystem skips signalling (for subsystems
// that are synchronously ready after Start returns nil).
type Subsystem interface {
	// Name returns a human-readable name for logging.
	Name() string

	// Start initialises the subsystem. It receives the NATS
	// client connection (may be nil for subsystems that don't
	// need NATS) and a ready channel for async readiness
	// signalling. Context cancellation signals shutdown.
	Start(ctx context.Context, nc *nats.Conn, ready chan<- error) error

	// Stop performs graceful shutdown. Called in reverse
	// registration order.
	Stop(ctx context.Context) error
}
