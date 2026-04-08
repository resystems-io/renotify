// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package broker

import "fmt"

// Flow-scoped NATS subject constructors. These match the subject
// catalogue in docs/analysis-nats-transport-design.md Section 1.1.

// FlowRequestSubject returns the JetStream subject for publishing
// a NotificationRequest.
func FlowRequestSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.request",
		username, flowID)
}

// FlowResponseSubject returns the JetStream subject for
// NotificationResponse messages.
func FlowResponseSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.response",
		username, flowID)
}

// FlowLifecycleSubject returns the JetStream subject for
// FlowLifecycleEvent messages.
func FlowLifecycleSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.lifecycle",
		username, flowID)
}

// FlowInterjectSubject returns the JetStream subject for
// InterjectionCommand messages.
func FlowInterjectSubject(username, flowID string) string {
	return fmt.Sprintf("resystems.renotify.%s.flow.%s.interject",
		username, flowID)
}

// ServiceFlowsSubject returns the Core NATS Request-Reply subject
// for the active flows query endpoint (R-CLI-14).
func ServiceFlowsSubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.svc.flows", username)
}

// ServiceHistorySubject returns the Core NATS Request-Reply
// subject for the notification history query endpoint (C-09).
func ServiceHistorySubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.svc.history", username)
}

// ServiceInsertRequestSubject returns the Core NATS
// Request-Reply subject for inserting a notification request
// into the ledger (C-17).
func ServiceInsertRequestSubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.svc.insert-request",
		username)
}

// ServiceInsertResponseSubject returns the Core NATS
// Request-Reply subject for inserting a notification response
// into the ledger (C-17).
func ServiceInsertResponseSubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.svc.insert-response",
		username)
}

// ServiceInsertInterjectionSubject returns the Core NATS
// Request-Reply subject for inserting an interjection audit
// record into the ledger (C-17).
func ServiceInsertInterjectionSubject(username string) string {
	return fmt.Sprintf(
		"resystems.renotify.%s.svc.insert-interjection",
		username)
}

// ServiceUpdateActivitySubject returns the Core NATS
// Request-Reply subject for updating a flow's last activity
// timestamp (C-17).
func ServiceUpdateActivitySubject(username string) string {
	return fmt.Sprintf(
		"resystems.renotify.%s.svc.update-activity",
		username)
}

// DeviceControlSubject returns the Core NATS Pub/Sub subject
// for sending control commands to a specific mobile device
// (C-16).
func DeviceControlSubject(username, deviceID string) string {
	return fmt.Sprintf("resystems.renotify.%s.device.%s.control",
		username, deviceID)
}

// DeviceHeartbeatSubject returns the Core NATS Pub/Sub subject
// for a mobile device's application-level heartbeat (R-MOB-14).
func DeviceHeartbeatSubject(username, deviceID string) string {
	return fmt.Sprintf("resystems.renotify.%s.device.%s.heartbeat",
		username, deviceID)
}

// ServiceDevicePresenceSubject returns the Core NATS
// Request-Reply subject for the device presence query
// endpoint (R-CLI-23).
func ServiceDevicePresenceSubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.svc.device-presence",
		username)
}

// MCP stdio relay subjects. These carry raw JSON-RPC messages
// between the `renotify mcp` CLI process and the daemon's
// mcp.Server via Core NATS Pub/Sub (ephemeral, not JetStream).

// MCPClientToServerSubject returns the subject the CLI
// publishes JSON-RPC messages to (stdin → NATS → daemon).
func MCPClientToServerSubject(username, sessionID string) string {
	return fmt.Sprintf("resystems.renotify.%s.mcp.%s.c2s",
		username, sessionID)
}

// MCPServerToClientSubject returns the subject the daemon
// publishes JSON-RPC messages to (daemon → NATS → stdout).
func MCPServerToClientSubject(username, sessionID string) string {
	return fmt.Sprintf("resystems.renotify.%s.mcp.%s.s2c",
		username, sessionID)
}

// MCPSessionOpenSubject returns the subject the CLI publishes
// to when a new stdio MCP session starts.
func MCPSessionOpenSubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.mcp.open", username)
}

// MCPSessionCloseSubject returns the subject the CLI publishes
// to when a stdio MCP session ends.
func MCPSessionCloseSubject(username string) string {
	return fmt.Sprintf("resystems.renotify.%s.mcp.close", username)
}
