package state

import (
	"github.com/google/uuid"

	"go.resystems.io/renotify/internal/crockford"
)

// GenerateFlowID creates a globally unique flow identifier.
// Format: fl_ + 26 Crockford Base32 chars (full 128-bit UUIDv7).
// UUIDv7 provides chronological sorting and global uniqueness.
//
// See docs/analysis-naming-and-addressing.md Section 2.5.
func GenerateFlowID() string {
	id := uuid.Must(uuid.NewV7())
	return "fl_" + crockford.EncodeBits(id[:], 128)
}

// GenerateNotificationID creates a globally unique notification
// identifier. Format: ntf_ + 16 Crockford Base32 chars (80 bits
// from UUIDv7, truncated).
//
// See docs/analysis-naming-and-addressing.md Section 3.
func GenerateNotificationID() string {
	id := uuid.Must(uuid.NewV7())
	return "ntf_" + crockford.EncodeBits(id[:], 80)
}
