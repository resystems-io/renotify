// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"strings"
	"testing"
)

func TestGenerateDeviceID_Format(t *testing.T) {
	id, err := GenerateDeviceID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "mb_") {
		t.Errorf("device_id should start with mb_, got %q", id)
	}
	// mb_ (3) + 13 Base32 chars = 16 total.
	if len(id) != 16 {
		t.Errorf("device_id length = %d, want 16", len(id))
	}
}

func TestGenerateDeviceID_Unique(t *testing.T) {
	id1, _ := GenerateDeviceID()
	id2, _ := GenerateDeviceID()
	if id1 == id2 {
		t.Error("two device IDs should be different")
	}
}

func TestGenerateDeviceToken_Format(t *testing.T) {
	tok, err := GenerateDeviceToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "rn_tk_") {
		t.Errorf("token should start with rn_tk_, got %q", tok)
	}
	// rn_tk_ (6) + 52 Base32 chars = 58 total.
	if len(tok) != 58 {
		t.Errorf("token length = %d, want 58", len(tok))
	}
}
