// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"strings"
	"testing"
)

func TestGenerateFlowID_Format(t *testing.T) {
	id := GenerateFlowID()
	if !strings.HasPrefix(id, "fl_") {
		t.Errorf("flow ID should start with fl_, got %q", id)
	}
	body := strings.TrimPrefix(id, "fl_")
	if len(body) != 26 {
		t.Errorf("body length = %d, want 26 (128 bits)", len(body))
	}
	for i, c := range body {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			t.Errorf("char %d %q not in Crockford alphabet", i, string(c))
		}
	}
}

func TestGenerateFlowID_Unique(t *testing.T) {
	id1 := GenerateFlowID()
	id2 := GenerateFlowID()
	if id1 == id2 {
		t.Errorf("two flow IDs should differ: %q", id1)
	}
}

func TestGenerateNotificationID_Format(t *testing.T) {
	id := GenerateNotificationID()
	if !strings.HasPrefix(id, "ntf_") {
		t.Errorf("notification ID should start with ntf_, got %q", id)
	}
	body := strings.TrimPrefix(id, "ntf_")
	if len(body) != 16 {
		t.Errorf("body length = %d, want 16 (80 bits)", len(body))
	}
	for i, c := range body {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			t.Errorf("char %d %q not in Crockford alphabet", i, string(c))
		}
	}
}

func TestGenerateNotificationID_Unique(t *testing.T) {
	id1 := GenerateNotificationID()
	id2 := GenerateNotificationID()
	if id1 == id2 {
		t.Errorf("two notification IDs should differ: %q", id1)
	}
}
