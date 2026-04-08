// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package heartbeat publishes periodic DaemonHeartbeat messages
// over Core NATS Pub/Sub. See
// docs/analysis-payload-schemas.md (DaemonHeartbeat) and
// docs/analysis-nats-transport-design.md Section 1.1.
package heartbeat

import (
	"fmt"
	"time"
)

// FlowInfo describes an active flow within a workspace
// heartbeat snapshot.
type FlowInfo struct {
	FlowID       string            `json:"flow_id"`
	Label        string            `json:"label,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	LastActivity time.Time         `json:"last_activity,omitempty"`
}

// WorkspaceInfo describes a single workspace within the
// daemon's heartbeat snapshot.
type WorkspaceInfo struct {
	WorkspaceID string     `json:"workspace_id"`
	DisplayName string     `json:"display_name"`
	AbsPath     string     `json:"abs_path"`
	ActiveFlows []FlowInfo `json:"active_flows"`
}

// DaemonHeartbeat is the periodic structural context published
// by each daemon instance. The mobile app uses it to build the
// dashboard hierarchy: flows → workspaces → daemons → hosts.
type DaemonHeartbeat struct {
	DaemonID                string          `json:"daemon_id"`
	Username                string          `json:"username"`
	Hostname                string          `json:"hostname"`
	GracePeriod             string          `json:"grace_period,omitempty"`
	DeviceHeartbeatInterval string          `json:"device_heartbeat_interval,omitempty"`
	Workspaces              []WorkspaceInfo `json:"workspaces"`
	Timestamp               time.Time       `json:"timestamp"`
}

// Subject returns the NATS subject for the heartbeat:
// resystems.renotify.{username}.daemon.{daemon_id}.heartbeat
func Subject(username, daemonID string) string {
	return fmt.Sprintf("resystems.renotify.%s.daemon.%s.heartbeat",
		username, daemonID)
}
