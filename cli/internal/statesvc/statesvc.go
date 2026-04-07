// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package statesvc defines the wire types for the daemon's state
// management NATS service endpoints (R-CLI-20). These types are
// shared between the state subsystem (which serves the endpoints)
// and any client (MCP server, CLI) that calls them.
//
// All endpoints use Core NATS Request-Reply on subjects under
// resystems.renotify.{username}.svc.*.
package statesvc

import (
	"time"

	"go.resystems.io/renotify/cli/internal/payload"
)

// --- Flow query (existing svc.flows) ---

// FlowsQuery holds optional filters for the svc.flows endpoint.
type FlowsQuery struct {
	FlowID      string `json:"flow_id,omitempty"`
	DaemonID    string `json:"daemon_id,omitempty"`
	WorkspaceID string `json:"workspace_id,omitempty"`
}

// FlowEntry is a single active flow returned by the svc.flows
// endpoint. Contains the fields needed by both the MCP server
// (for flow context lookups) and the CLI (for display).
type FlowEntry struct {
	FlowID                string            `json:"flow_id"`
	DaemonID              string            `json:"daemon_id"`
	WorkspaceID           string            `json:"workspace_id"`
	DisplayName           string            `json:"display_name,omitempty"`
	AbsPath               string            `json:"abs_path,omitempty"`
	Label                 string            `json:"label,omitempty"`
	Metadata              map[string]string `json:"metadata,omitempty"`
	RegisteredAt          time.Time         `json:"registered_at"`
	LastActivityTimestamp time.Time         `json:"last_activity_timestamp"`
}

// FlowsResult is the response payload for the svc.flows endpoint.
type FlowsResult struct {
	Flows []FlowEntry `json:"flows"`
}

// --- History query (existing svc.history) ---

// HistoryQueryRequest holds optional filters for the svc.history
// endpoint. All fields are optional; when omitted the daemon
// returns the most recent records up to the default limit.
type HistoryQueryRequest struct {
	WorkspaceID string     `json:"workspace_id,omitempty"`
	FlowID      string     `json:"flow_id,omitempty"`
	Since       *time.Time `json:"since,omitempty"`
	Until       *time.Time `json:"until,omitempty"`
	Limit       int        `json:"limit,omitempty"`
	Offset      int        `json:"offset,omitempty"`
}

// HistoryRecord represents a single entry in the unified history
// timeline. It is either a notification (Request populated) or a
// lifecycle event (Lifecycle populated), discriminated by Type.
type HistoryRecord struct {
	Type          string                        `json:"type"`
	Username      string                        `json:"username"`
	FlowLabel     string                        `json:"flow_label,omitempty"`
	WorkspaceName string                        `json:"workspace_name,omitempty"`
	WorkspacePath string                        `json:"workspace_path,omitempty"`
	Request       *payload.NotificationRequest  `json:"request,omitempty"`
	Response      *payload.NotificationResponse `json:"response,omitempty"`
	Lifecycle     *payload.FlowLifecycleEvent   `json:"lifecycle,omitempty"`
}

// HistoryQueryResult is the response payload for the svc.history
// endpoint.
type HistoryQueryResult struct {
	Records []HistoryRecord `json:"records"`
	Total   int             `json:"total"`
}

// --- Write endpoints (new for C-17) ---

// InsertRequestCmd is the request payload for the
// svc.insert-request endpoint.
type InsertRequestCmd struct {
	Username      string                      `json:"username"`
	FlowLabel     string                      `json:"flow_label,omitempty"`
	WorkspaceName string                      `json:"workspace_name,omitempty"`
	WorkspacePath string                      `json:"workspace_path,omitempty"`
	Request       payload.NotificationRequest `json:"request"`
}

// InsertResponseCmd is the request payload for the
// svc.insert-response endpoint.
type InsertResponseCmd struct {
	Response payload.NotificationResponse `json:"response"`
}

// InsertInterjectionCmd is the request payload for the
// svc.insert-interjection endpoint.
type InsertInterjectionCmd struct {
	Username     string                      `json:"username"`
	Interjection payload.InterjectionCommand `json:"interjection"`
}

// UpdateActivityCmd is the request payload for the
// svc.update-activity endpoint.
type UpdateActivityCmd struct {
	FlowID    string    `json:"flow_id"`
	Timestamp time.Time `json:"timestamp"`
}

// WriteResult is the generic response for write endpoints.
type WriteResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}
