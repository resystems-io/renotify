// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package payload

import "time"

// ErrorResponse is the generic error envelope returned when any
// request fails at the daemon level.
type ErrorResponse struct {
	CorrelationID string    `json:"correlation_id,omitempty"`
	Code          string    `json:"code"`
	Message       string    `json:"message"`
	Timestamp     time.Time `json:"timestamp"`
}
