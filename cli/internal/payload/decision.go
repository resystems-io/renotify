// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package payload

import "time"

// DecisionResource is the MCP dynamic resource that agents read
// after receiving a notifications/resources/updated event
// (R-CLI-10). While decided is false, the response fields are
// absent. Once decided, the resource is immutable.
type DecisionResource struct {
	RequestID string    `json:"request_id"`
	Decided   bool      `json:"decided"`
	Accepted  *bool     `json:"accepted,omitempty"`
	Action    string    `json:"action,omitempty"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}
