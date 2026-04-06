// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package payload defines the shared domain types serialised
// to/from NATS messages. These are the wire-format types used
// across CLI commands, the daemon, MCP server, and the Android
// app. See docs/analysis-payload-schemas.md.
package payload

// ResponseType indicates what kind of human feedback a
// notification expects.
type ResponseType string

const (
	ResponseNone    ResponseType = "none"
	ResponseBoolean ResponseType = "boolean"
	ResponseChoice  ResponseType = "choice"
	ResponseText    ResponseType = "text"
)

// Priority controls how prominently the Android app renders the
// notification.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

// FlowStatus represents the current lifecycle state of a
// pipeline flow.
type FlowStatus string

const (
	FlowActive    FlowStatus = "active"
	FlowCompleted FlowStatus = "completed"
	FlowFailed    FlowStatus = "failed"
)

// InterjectionAction is the type of proactive control signal
// from the mobile client.
type InterjectionAction string

const (
	InterjectionStop  InterjectionAction = "stop"
	InterjectionPause InterjectionAction = "pause"
	InterjectionNote  InterjectionAction = "note"
)
