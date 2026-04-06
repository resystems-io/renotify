// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.resystems.io/renotify/cli/internal/crockford"
)

func TestLoadOrGenerateDaemonID_GeneratesOnMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon_id")
	id, err := LoadOrGenerateDaemonID(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "dn_") {
		t.Errorf("daemon_id should start with dn_, got %q", id)
	}
	body := strings.TrimPrefix(id, "dn_")
	if len(body) != 13 {
		t.Errorf("daemon_id body length = %d, want 13", len(body))
	}
}

func TestLoadOrGenerateDaemonID_LoadsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon_id")
	os.WriteFile(path, []byte("dn_EXISTINGVALUE\n"), 0600)

	id, err := LoadOrGenerateDaemonID(path)
	if err != nil {
		t.Fatal(err)
	}
	if id != "dn_EXISTINGVALUE" {
		t.Errorf("got %q, want %q", id, "dn_EXISTINGVALUE")
	}
}

func TestLoadOrGenerateDaemonID_CreatesDirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "c")
	path := filepath.Join(dir, "daemon_id")
	_, err := LoadOrGenerateDaemonID(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("parent dirs should be created")
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("dir perm = %o, want 0700", perm)
	}
}

func TestLoadOrGenerateDaemonID_Format(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon_id")
	id, err := LoadOrGenerateDaemonID(path)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.TrimPrefix(id, "dn_")
	// Verify all chars are in Crockford alphabet.
	for i, c := range body {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			t.Errorf("char %d %q not in Crockford alphabet", i, string(c))
		}
	}
	// Verify it decodes without error.
	_, err = crockford.Decode(body)
	if err != nil {
		t.Errorf("body is not valid Crockford: %v", err)
	}
}

func TestLoadOrGenerateDaemonID_FilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon_id")
	_, err := LoadOrGenerateDaemonID(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
}
