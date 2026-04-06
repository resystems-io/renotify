// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package payload

import "time"

// NotificationRequest is the core domain payload representing an
// interrupt or alert sent from a CLI command or AI agent to the
// Android app. See docs/analysis-payload-schemas.md.
type NotificationRequest struct {
	ID            string         `json:"id"`
	FlowID        string         `json:"flow_id"`
	DaemonID      string         `json:"daemon_id"`
	WorkspaceID   string         `json:"workspace_id"`
	Title         string         `json:"title"`
	Body          string         `json:"body,omitempty"`
	ResponseTypes []ResponseType `json:"response_types"`
	Priority      Priority       `json:"priority"`
	Source        string         `json:"source"`
	WorkspaceName string         `json:"workspace_name,omitempty"`
	Actions       []string       `json:"actions,omitempty"`
	TimeoutSec    int            `json:"timeout_sec,omitempty"`
	Timestamp     time.Time      `json:"timestamp"`
}

// NotificationResponse is the human decision returned from the
// Android app, correlated to a NotificationRequest by RequestID.
type NotificationResponse struct {
	RequestID string    `json:"request_id"`
	Accepted  *bool     `json:"accepted,omitempty"`
	Action    string    `json:"action,omitempty"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
