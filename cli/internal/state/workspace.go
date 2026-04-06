// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"crypto/sha256"
	"path"

	"go.resystems.io/renotify/cli/internal/crockford"
)

// WorkspaceID computes a deterministic workspace identifier from
// a daemon ID and absolute path. The result is globally unique
// because daemon_id is globally unique and the absolute path is
// unique per filesystem.
//
// Formula: ws_ + first 16 Crockford Base32 chars (80 bits) of
// SHA-256(daemonID + "|" + absPath).
//
// See docs/analysis-naming-and-addressing.md Section 2.4.
func WorkspaceID(daemonID, absPath string) string {
	h := sha256.Sum256([]byte(daemonID + "|" + absPath))
	enc, _ := crockford.EncodeBits(h[:], 80) // cannot fail: 80 < 256
	return "ws_" + enc
}

// DisplayName returns a human-readable workspace name derived
// from the directory basename. Not unique — used for UI display
// only.
func DisplayName(absPath string) string {
	return path.Base(absPath)
}
