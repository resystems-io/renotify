package broker

import (
	"strings"
	"testing"
)

func TestBuildAuth_DaemonPermissions(t *testing.T) {
	users := BuildAuthConfig("alice", "internal_tok", "")
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
	users := BuildAuthConfig("alice", "internal_tok", "pairing_tok")
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	mobile := users[1]
	if mobile.Username != "mobile" {
		t.Errorf("username = %q, want %q", mobile.Username, "mobile")
	}
	if mobile.Password != "pairing_tok" {
		t.Errorf("password = %q, want %q", mobile.Password, "pairing_tok")
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

func TestBuildAuth_NoPairingToken(t *testing.T) {
	users := BuildAuthConfig("bob", "internal_tok", "")
	if len(users) != 1 {
		t.Errorf("expected 1 user (daemon only), got %d", len(users))
	}
}

func TestBuildAuth_MobileCannotPublishRequest(t *testing.T) {
	users := BuildAuthConfig("alice", "internal_tok", "pairing_tok")
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
