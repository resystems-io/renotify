// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package presence

import (
	"context"
	"log/slog"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natstest "github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/state"
)

const testUsername = "alice"

func startTestServer(t *testing.T) *nats.Conn {
	t.Helper()
	opts := natstest.DefaultTestOptions
	opts.Port = -1
	srv := natstest.RunServer(&opts)
	t.Cleanup(srv.Shutdown)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func startTracker(
	t *testing.T,
	nc *nats.Conn,
	staleThreshold time.Duration,
	devices []state.PairedDevice,
) *Tracker {
	t.Helper()
	tr := New(testUsername, staleThreshold, devices, discardLogger())
	ready := make(chan error, 1)
	if err := tr.Start(context.Background(), nc, ready); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tracker start timeout")
	}
	t.Cleanup(func() { tr.Stop(context.Background()) })
	return tr
}

func publishHeartbeat(t *testing.T, nc *nats.Conn, deviceID string) {
	t.Helper()
	subject := "resystems.renotify." + testUsername +
		".device." + deviceID + ".heartbeat"
	data := []byte(`{"device_id":"` + deviceID +
		`","timestamp":"2026-04-08T10:00:00Z"}`)
	if err := nc.Publish(subject, data); err != nil {
		t.Fatal(err)
	}
	nc.Flush()
}

func TestTracker_OnlineAfterHeartbeat(t *testing.T) {
	nc := startTestServer(t)
	devices := []state.PairedDevice{
		{DeviceID: "mb_DEV01", PairedAt: time.Now()},
	}
	tr := startTracker(t, nc, 5*time.Second, devices)

	publishHeartbeat(t, nc, "mb_DEV01")
	// Allow the subscription handler to fire.
	time.Sleep(50 * time.Millisecond)

	statuses := tr.DevicePresence()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 device, got %d", len(statuses))
	}
	if !statuses[0].Online {
		t.Error("device should be online after heartbeat")
	}
	if statuses[0].LastSeen == nil {
		t.Error("last_seen should be set")
	}
	if statuses[0].Username != testUsername {
		t.Errorf("username = %q, want %q",
			statuses[0].Username, testUsername)
	}
}

func TestTracker_OfflineAfterStaleThreshold(t *testing.T) {
	nc := startTestServer(t)
	devices := []state.PairedDevice{
		{DeviceID: "mb_DEV01", PairedAt: time.Now()},
	}
	// Short stale threshold for testing.
	tr := startTracker(t, nc, 200*time.Millisecond, devices)

	publishHeartbeat(t, nc, "mb_DEV01")
	time.Sleep(50 * time.Millisecond)

	// Should be online immediately after heartbeat.
	statuses := tr.DevicePresence()
	if !statuses[0].Online {
		t.Error("device should be online immediately after heartbeat")
	}

	// Wait past the stale threshold.
	time.Sleep(300 * time.Millisecond)

	statuses = tr.DevicePresence()
	if statuses[0].Online {
		t.Error("device should be offline after stale threshold")
	}
	if statuses[0].LastSeen == nil {
		t.Error("last_seen should still be set")
	}
}

func TestTracker_NeverSeenDevice(t *testing.T) {
	nc := startTestServer(t)
	devices := []state.PairedDevice{
		{DeviceID: "mb_DEV01", PairedAt: time.Now()},
	}
	tr := startTracker(t, nc, 5*time.Second, devices)

	statuses := tr.DevicePresence()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 device, got %d", len(statuses))
	}
	if statuses[0].Online {
		t.Error("never-seen device should be offline")
	}
	if statuses[0].LastSeen != nil {
		t.Error("last_seen should be nil for never-seen device")
	}
}

func TestTracker_MultipleDevices(t *testing.T) {
	nc := startTestServer(t)
	devices := []state.PairedDevice{
		{DeviceID: "mb_DEV01", PairedAt: time.Now()},
		{DeviceID: "mb_DEV02", PairedAt: time.Now()},
	}
	tr := startTracker(t, nc, 5*time.Second, devices)

	// Only send heartbeat for device 1.
	publishHeartbeat(t, nc, "mb_DEV01")
	time.Sleep(50 * time.Millisecond)

	statuses := tr.DevicePresence()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(statuses))
	}

	byID := make(map[string]bool)
	for _, s := range statuses {
		byID[s.DeviceID] = s.Online
	}
	if !byID["mb_DEV01"] {
		t.Error("mb_DEV01 should be online")
	}
	if byID["mb_DEV02"] {
		t.Error("mb_DEV02 should be offline")
	}
}

func TestTracker_ReloadDevices(t *testing.T) {
	nc := startTestServer(t)
	devices := []state.PairedDevice{
		{DeviceID: "mb_DEV01", PairedAt: time.Now()},
	}
	tr := startTracker(t, nc, 5*time.Second, devices)

	// Send heartbeat so DEV01 has a last-seen time.
	publishHeartbeat(t, nc, "mb_DEV01")
	time.Sleep(50 * time.Millisecond)

	// Reload with DEV01 still present and DEV02 added.
	tr.ReloadDevices([]state.PairedDevice{
		{DeviceID: "mb_DEV01", PairedAt: time.Now()},
		{DeviceID: "mb_DEV02", PairedAt: time.Now()},
	})

	statuses := tr.DevicePresence()
	if len(statuses) != 2 {
		t.Fatalf("expected 2 devices after reload, got %d",
			len(statuses))
	}

	// DEV01 should retain its last-seen.
	for _, s := range statuses {
		if s.DeviceID == "mb_DEV01" && s.LastSeen == nil {
			t.Error("mb_DEV01 should retain last_seen after reload")
		}
		if s.DeviceID == "mb_DEV02" && s.LastSeen != nil {
			t.Error("mb_DEV02 should have nil last_seen")
		}
	}

	// Reload removing DEV01.
	tr.ReloadDevices([]state.PairedDevice{
		{DeviceID: "mb_DEV02", PairedAt: time.Now()},
	})

	statuses = tr.DevicePresence()
	if len(statuses) != 1 {
		t.Fatalf("expected 1 device after revoke, got %d",
			len(statuses))
	}
	if statuses[0].DeviceID != "mb_DEV02" {
		t.Errorf("remaining device = %q, want mb_DEV02",
			statuses[0].DeviceID)
	}
}

func TestExtractDeviceID(t *testing.T) {
	tests := []struct {
		subject string
		want    string
	}{
		{
			"resystems.renotify.alice.device.mb_DEV01.heartbeat",
			"mb_DEV01",
		},
		{
			"resystems.renotify.bob.device.mb_XYZ.heartbeat",
			"mb_XYZ",
		},
		{
			"resystems.renotify.alice.flow.fl_TEST.request",
			"",
		},
		{
			"invalid",
			"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.subject, func(t *testing.T) {
			got := extractDeviceID(tc.subject)
			if got != tc.want {
				t.Errorf("extractDeviceID(%q) = %q, want %q",
					tc.subject, got, tc.want)
			}
		})
	}
}

// startTestServer is imported via natstest; ensure import is used.
var _ *natsserver.Server
