// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package presence tracks mobile device connectivity by
// subscribing to application-level heartbeats (R-CLI-23,
// R-MOB-14). See docs/analysis-device-presence.md.
package presence

import "time"

// DeviceHeartbeat is the JSON payload published by the mobile
// app on its device-specific heartbeat subject.
type DeviceHeartbeat struct {
	DeviceID  string    `json:"device_id"`
	Timestamp time.Time `json:"timestamp"`
}
