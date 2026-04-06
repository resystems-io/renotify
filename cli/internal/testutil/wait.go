// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package testutil provides shared test helpers.
package testutil

import (
	"testing"
	"time"
)

// WaitFor polls fn every 20ms until it returns true or timeout
// expires. Returns true if the condition was met before the
// deadline. Use this instead of time.Sleep to make tests
// event-driven.
func WaitFor(t *testing.T, timeout time.Duration, fn func() bool) bool {
	t.Helper()
	deadline := time.After(timeout)
	for {
		if fn() {
			return true
		}
		select {
		case <-deadline:
			return false
		case <-time.After(20 * time.Millisecond):
		}
	}
}
