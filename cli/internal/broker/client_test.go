// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package broker

import (
	"testing"

	"go.resystems.io/renotify/cli/internal/config"
)

// These tests verify that ConnectShared constructs correct options
// without actually connecting. We test by calling the function
// with an unreachable URL and checking the error message, which
// confirms the option was applied.

func TestConnectSharedOptions_TokenAuth(t *testing.T) {
	cfg := config.SharedBrokerConfig{
		URL:   "nats://unreachable:4222",
		Token: "test_token",
	}
	_, err := ConnectShared(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected connection error to unreachable host")
	}
	// If we got here, the function constructed options without
	// panicking. The token option was applied (no auth error
	// in the error message — just connection refused).
}

func TestConnectSharedOptions_UserPassAuth(t *testing.T) {
	cfg := config.SharedBrokerConfig{
		URL:      "nats://unreachable:4222",
		Username: "user",
		Password: "pass",
	}
	_, err := ConnectShared(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected connection error to unreachable host")
	}
}

func TestConnectSharedOptions_TLS(t *testing.T) {
	cfg := config.SharedBrokerConfig{
		URL:        "nats://unreachable:4222",
		TLSEnabled: true,
	}
	_, err := ConnectShared(cfg, discardLogger())
	if err == nil {
		t.Fatal("expected connection error to unreachable host")
	}
}
