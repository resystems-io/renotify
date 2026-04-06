// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package broker

import (
	"strings"
	"testing"

	"go.resystems.io/renotify/cli/internal/state"
)

func TestBuildAuth_DaemonPermissions(t *testing.T) {
	users := BuildAuthConfig("alice", "internal_tok", nil)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	u := users[0]
	if u.Username != "daemon" {
		t.Errorf("username = %q, want %q", u.Username, "daemon")
	}
	if u.Password != "internal_tok" {
		t.Errorf("password = %q, want %q", u.Password, "internal_tok")
	}

	// Check publish permissions.
	pub := u.Permissions.Publish.Allow
	assertContains(t, pub, "resystems.renotify.alice.>", "daemon publish")
	assertContains(t, pub, "$JS.API.>", "daemon publish")

	// Check subscribe permissions.
	sub := u.Permissions.Subscribe.Allow
	assertContains(t, sub, "resystems.renotify.alice.>", "daemon subscribe")
	assertContains(t, sub, "$JS.API.>", "daemon subscribe")
	assertContains(t, sub, "_INBOX.>", "daemon subscribe")
}

func TestBuildAuth_MobilePermissions(t *testing.T) {
	devices := []state.PairedDevice{{
		DeviceID: "mb_TESTDEV01",
		Token:    "rn_tk_TESTTOKEN",
	}}
	users := BuildAuthConfig("alice", "internal_tok", devices)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	mobile := users[1]
	wantUser := "mobile-mb_TESTDEV01"
	if mobile.Username != wantUser {
		t.Errorf("username = %q, want %q",
			mobile.Username, wantUser)
	}
	if mobile.Password != "rn_tk_TESTTOKEN" {
		t.Errorf("password = %q", mobile.Password)
	}

	pub := mobile.Permissions.Publish.Allow
	assertContains(t, pub, "resystems.renotify.alice.flow.*.response", "mobile publish")
	assertContains(t, pub, "resystems.renotify.alice.flow.*.interject", "mobile publish")
	assertContains(t, pub, "resystems.renotify.alice.svc.*", "mobile publish")
	assertContains(t, pub, "$JS.ACK.>", "mobile publish")
	assertContains(t, pub, "$JS.FC.>", "mobile publish")

	sub := mobile.Permissions.Subscribe.Allow
	assertContains(t, sub, "resystems.renotify.alice.>", "mobile subscribe")
	assertContains(t, sub, "_INBOX.>", "mobile subscribe")
}

func TestBuildAuth_NoDevices(t *testing.T) {
	users := BuildAuthConfig("bob", "internal_tok", nil)
	if len(users) != 1 {
		t.Errorf("expected 1 user (daemon only), got %d",
			len(users))
	}
}

func TestBuildAuth_MultipleDevices(t *testing.T) {
	devices := []state.PairedDevice{
		{DeviceID: "mb_DEV01", Token: "rn_tk_TOK01"},
		{DeviceID: "mb_DEV02", Token: "rn_tk_TOK02"},
	}
	users := BuildAuthConfig("alice", "internal_tok", devices)
	if len(users) != 3 {
		t.Fatalf("expected 3 users (daemon + 2 devices), got %d",
			len(users))
	}
	if users[1].Username != "mobile-mb_DEV01" {
		t.Errorf("user[1] = %q", users[1].Username)
	}
	if users[2].Username != "mobile-mb_DEV02" {
		t.Errorf("user[2] = %q", users[2].Username)
	}
}

func TestBuildAuth_MobileCannotPublishRequest(t *testing.T) {
	devices := []state.PairedDevice{{
		DeviceID: "mb_TESTDEV01",
		Token:    "rn_tk_TESTTOKEN",
	}}
	users := BuildAuthConfig("alice", "internal_tok", devices)
	mobile := users[1]
	pub := mobile.Permissions.Publish.Allow
	for _, s := range pub {
		if strings.Contains(s, ".request") {
			t.Errorf("mobile should not be able to publish to .request, found %q", s)
		}
		if strings.Contains(s, ".lifecycle") {
			t.Errorf("mobile should not be able to publish to .lifecycle, found %q", s)
		}
		if strings.Contains(s, ".heartbeat") {
			t.Errorf("mobile should not be able to publish to .heartbeat, found %q", s)
		}
	}
}

func assertContains(t *testing.T, subjects []string, want, context string) {
	t.Helper()
	for _, s := range subjects {
		if s == want {
			return
		}
	}
	t.Errorf("%s: %q not found in %v", context, want, subjects)
}
