// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package presence

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/state"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

// deviceState tracks the last-seen time for a single device.
type deviceState struct {
	DeviceID string
	PairedAt time.Time
	LastSeen time.Time // zero value = never seen
}

// Tracker is a daemon.Subsystem that tracks mobile device
// connectivity by subscribing to device heartbeats. It exposes
// a DevicePresence() method that returns the current state of
// all paired devices.
type Tracker struct {
	username       string
	staleThreshold time.Duration
	logger         *slog.Logger

	nc  *nats.Conn
	sub *nats.Subscription

	mu      sync.RWMutex
	devices map[string]*deviceState // keyed by device_id
}

// New creates a presence Tracker. The staleThreshold controls
// how long after the last heartbeat a device is considered
// offline. The devices slice initialises the device map from
// the current pairing registry.
func New(
	username string,
	staleThreshold time.Duration,
	devices []state.PairedDevice,
	logger *slog.Logger,
) *Tracker {
	t := &Tracker{
		username:       username,
		staleThreshold: staleThreshold,
		logger:         logger,
		devices:        make(map[string]*deviceState, len(devices)),
	}
	for _, d := range devices {
		t.devices[d.DeviceID] = &deviceState{
			DeviceID: d.DeviceID,
			PairedAt: d.PairedAt,
		}
	}
	return t
}

// Name implements daemon.Subsystem.
func (t *Tracker) Name() string { return "presence" }

// Start implements daemon.Subsystem. It subscribes to all device
// heartbeats via a wildcard subject.
func (t *Tracker) Start(
	_ context.Context,
	nc *nats.Conn,
	ready chan<- error,
) error {
	t.nc = nc

	// Subscribe to all device heartbeats:
	// resystems.renotify.{username}.device.*.heartbeat
	subject := fmt.Sprintf("resystems.renotify.%s.device.*.heartbeat",
		t.username)
	sub, err := nc.Subscribe(subject, t.handleHeartbeat)
	if err != nil {
		if ready != nil {
			ready <- err
			close(ready)
		}
		return err
	}
	t.sub = sub

	t.logger.Info("presence tracker started",
		"subject", subject,
		"devices", len(t.devices))

	if ready != nil {
		close(ready)
	}
	return nil
}

// Stop implements daemon.Subsystem.
func (t *Tracker) Stop(_ context.Context) error {
	if t.sub != nil {
		t.sub.Drain()
	}
	t.logger.Info("presence tracker stopped")
	return nil
}

// handleHeartbeat processes an incoming device heartbeat. The
// device ID is extracted from the NATS subject rather than
// parsing the JSON payload, avoiding unnecessary allocation on
// the hot path.
func (t *Tracker) handleHeartbeat(msg *nats.Msg) {
	deviceID := extractDeviceID(msg.Subject)
	if deviceID == "" {
		return
	}

	now := time.Now().UTC()

	t.mu.Lock()
	ds, ok := t.devices[deviceID]
	if ok {
		ds.LastSeen = now
	}
	t.mu.Unlock()

	if !ok {
		t.logger.Warn("heartbeat from unknown device",
			"device_id", deviceID)
	}
}

// DevicePresence returns the current presence state of all
// paired devices.
func (t *Tracker) DevicePresence() []statesvc.DeviceStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now().UTC()
	result := make([]statesvc.DeviceStatus, 0, len(t.devices))

	for _, ds := range t.devices {
		status := statesvc.DeviceStatus{
			Username: t.username,
			DeviceID: ds.DeviceID,
			PairedAt: ds.PairedAt,
		}
		if !ds.LastSeen.IsZero() {
			ls := ds.LastSeen
			status.LastSeen = &ls
			status.Online = now.Sub(ds.LastSeen) < t.staleThreshold
		}
		result = append(result, status)
	}

	return result
}

// ReloadDevices updates the device map after a pair/revoke
// operation. Existing devices retain their last-seen state.
// New devices start with zero last-seen. Revoked devices are
// removed.
func (t *Tracker) ReloadDevices(devices []state.PairedDevice) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Build a set of current device IDs.
	current := make(map[string]struct{}, len(devices))
	for _, d := range devices {
		current[d.DeviceID] = struct{}{}
		if _, ok := t.devices[d.DeviceID]; !ok {
			t.devices[d.DeviceID] = &deviceState{
				DeviceID: d.DeviceID,
				PairedAt: d.PairedAt,
			}
		}
	}

	// Remove revoked devices.
	for id := range t.devices {
		if _, ok := current[id]; !ok {
			delete(t.devices, id)
		}
	}
}

// extractDeviceID extracts the device ID from a heartbeat
// subject of the form:
//
//	resystems.renotify.{username}.device.{device_id}.heartbeat
//
// Returns "" if the subject doesn't have the expected structure.
func extractDeviceID(subject string) string {
	// Find the segment between "device." and ".heartbeat".
	const prefix = ".device."
	i := strings.Index(subject, prefix)
	if i < 0 {
		return ""
	}
	rest := subject[i+len(prefix):]
	j := strings.IndexByte(rest, '.')
	if j < 0 {
		return ""
	}
	return rest[:j]
}
