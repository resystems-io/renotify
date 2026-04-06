// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"github.com/google/uuid"

	"go.resystems.io/renotify/cli/internal/crockford"
)

// GenerateFlowID creates a globally unique flow identifier.
// Format: fl_ + 26 Crockford Base32 chars (full 128-bit UUIDv7).
// UUIDv7 provides chronological sorting and global uniqueness.
//
// See docs/analysis-naming-and-addressing.md Section 2.5.
func GenerateFlowID() string {
	id := uuid.Must(uuid.NewV7())
	enc, _ := crockford.EncodeBits(id[:], 128) // cannot fail: 128 == 16*8
	return "fl_" + enc
}

// GenerateNotificationID creates a globally unique notification
// identifier. Format: ntf_ + 16 Crockford Base32 chars (80 bits
// from UUIDv7, truncated).
//
// See docs/analysis-naming-and-addressing.md Section 3.
func GenerateNotificationID() string {
	id := uuid.Must(uuid.NewV7())
	enc, _ := crockford.EncodeBits(id[:], 80) // cannot fail: 80 < 128
	return "ntf_" + enc
}
