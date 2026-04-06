// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package state manages persistent daemon state files under
// $XDG_STATE_HOME/renotify/. See docs/analysis-configuration-schema.md
// Section 1.
package state

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.resystems.io/renotify/cli/internal/crockford"
)

// LoadOrGenerateDaemonID reads the daemon_id from path. If the
// file does not exist, it generates a new daemon_id (dn_ + 13
// Crockford Base32 chars from 65 random bits), writes it
// atomically, and returns it.
func LoadOrGenerateDaemonID(path string) (string, error) {
	return loadOrGenerate(path, func() (string, error) {
		b := make([]byte, 9) // 72 bits; we use 65
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("generate daemon_id: %w", err)
		}
		enc, err := crockford.EncodeBits(b, 65)
		if err != nil {
			return "", fmt.Errorf("generate daemon_id: %w", err)
		}
		return "dn_" + enc, nil
	})
}

// loadOrGenerate is the shared load-or-generate-and-persist
// pattern used by daemon_id and token files.
func loadOrGenerate(path string, generate func() (string, error)) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	val, err := generate()
	if err != nil {
		return "", err
	}

	if err := writeAtomic(path, val); err != nil {
		return "", err
	}
	return val, nil
}

// writeAtomic writes val to path atomically by writing to a
// temporary file and renaming. Creates parent directories with
// 0700. The file is written with mode 0600.
func writeAtomic(path, val string) error {
	return writeAtomicBytes(path, []byte(val+"\n"), 0600)
}

// writeAtomicBytes writes data to path atomically with the
// specified permissions. Creates parent directories with 0700.
func writeAtomicBytes(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}
