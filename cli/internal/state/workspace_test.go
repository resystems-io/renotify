// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"strings"
	"testing"
)

func TestWorkspaceID_Deterministic(t *testing.T) {
	id1 := WorkspaceID("dn_3G2K7V9WNFQ4J", "/home/alice/projects/renotify")
	id2 := WorkspaceID("dn_3G2K7V9WNFQ4J", "/home/alice/projects/renotify")
	if id1 != id2 {
		t.Errorf("same inputs produced different IDs: %q vs %q", id1, id2)
	}
}

func TestWorkspaceID_Format(t *testing.T) {
	id := WorkspaceID("dn_3G2K7V9WNFQ4J", "/home/alice/projects/renotify")
	if !strings.HasPrefix(id, "ws_") {
		t.Errorf("workspace ID should start with ws_, got %q", id)
	}
	body := strings.TrimPrefix(id, "ws_")
	if len(body) != 16 {
		t.Errorf("body length = %d, want 16", len(body))
	}
	for i, c := range body {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			t.Errorf("char %d %q not in Crockford alphabet", i, string(c))
		}
	}
}

func TestWorkspaceID_DifferentInputs(t *testing.T) {
	id1 := WorkspaceID("dn_3G2K7V9WNFQ4J", "/home/alice/projects/renotify")
	id2 := WorkspaceID("dn_3G2K7V9WNFQ4J", "/home/alice/projects/other")
	if id1 == id2 {
		t.Error("different paths should produce different IDs")
	}

	id3 := WorkspaceID("dn_DIFFERENT0001", "/home/alice/projects/renotify")
	if id1 == id3 {
		t.Error("different daemon IDs should produce different IDs")
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/home/alice/projects/renotify", "renotify"},
		{"/opt/builds/my-project", "my-project"},
		{"/", "/"},
	}
	for _, tc := range tests {
		got := DisplayName(tc.path)
		if got != tc.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
