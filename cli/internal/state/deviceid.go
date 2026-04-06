// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"crypto/rand"
	"fmt"

	"go.resystems.io/renotify/cli/internal/crockford"
)

// GenerateDeviceID generates a new device_id: mb_ + 13
// Crockford Base32 chars from 65 random bits. A fresh ID is
// generated for each pairing — not persisted daemon-side.
func GenerateDeviceID() (string, error) {
	b := make([]byte, 9) // 72 bits; we use 65
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate device_id: %w", err)
	}
	enc, err := crockford.EncodeBits(b, 65)
	if err != nil {
		return "", fmt.Errorf("generate device_id: %w", err)
	}
	return "mb_" + enc, nil
}

// GenerateDeviceToken generates a new per-device auth token
// (rn_tk_ + 52 Crockford Base32 chars from 256 random bits).
func GenerateDeviceToken() (string, error) {
	b := make([]byte, 32) // 256 bits
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate device token: %w", err)
	}
	enc, err := crockford.EncodeBits(b, 256)
	if err != nil {
		return "", fmt.Errorf("generate device token: %w", err)
	}
	return "rn_tk_" + enc, nil
}
