package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/state"
)

func TestSilent_RequiresArg(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")

	_, _, err := executeCommand("silent")
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

func TestSilent_InvalidArg(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	_, _, err := executeCommand("silent", "maybe")
	if err == nil {
		t.Fatal("expected error for invalid arg")
	}
	if !strings.Contains(err.Error(), "'on' or 'off'") {
		t.Errorf("error = %q, want usage hint", err)
	}
}

func TestSilent_NoDevices(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", stateDir)

	_, _, err := executeCommand("silent", "on")
	if err == nil {
		t.Fatal("expected error for no devices")
	}
	if !strings.Contains(err.Error(), "no paired devices") {
		t.Errorf("error = %q, want 'no paired devices'", err)
	}
}

func TestSilent_DeviceNotFound(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", stateDir)

	// Create a device.
	devicesPath := filepath.Join(stateDir, "renotify",
		"pairing", "devices.json")
	os.MkdirAll(filepath.Dir(devicesPath), 0o755)
	state.AddDevice(devicesPath, state.PairedDevice{
		DeviceID: "mb_AAAAAAAAAAAAA",
		Token:    "rn_tk_test",
		PairedAt: time.Now().UTC(),
	})

	_, _, err := executeCommand("silent",
		"--device", "mb_NONEXISTENT", "on")
	if err == nil {
		t.Fatal("expected error for device not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err)
	}
}

func TestSilent_MultipleDevicesRequiresFlag(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", stateDir)

	devicesPath := filepath.Join(stateDir, "renotify",
		"pairing", "devices.json")
	os.MkdirAll(filepath.Dir(devicesPath), 0o755)
	for _, id := range []string{
		"mb_AAAAAAAAAAAAA", "mb_BBBBBBBBBBBBB",
	} {
		state.AddDevice(devicesPath, state.PairedDevice{
			DeviceID: id,
			Token:    "rn_tk_test",
			PairedAt: time.Now().UTC(),
		})
	}

	_, _, err := executeCommand("silent", "on")
	if err == nil {
		t.Fatal("expected error for ambiguous device")
	}
	if !strings.Contains(err.Error(), "--device or --all") {
		t.Errorf("error = %q, want '--device or --all'", err)
	}
}

func TestSilent_PublishesToDevice(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	devicesPath := filepath.Join(stateDir, "renotify",
		"pairing", "devices.json")
	os.MkdirAll(filepath.Dir(devicesPath), 0o755)
	state.AddDevice(devicesPath, state.PairedDevice{
		DeviceID: "mb_TESTDEV00001",
		Token:    "rn_tk_test",
		PairedAt: time.Now().UTC(),
	})

	// Subscribe to capture the control message.
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	msgCh := make(chan *nats.Msg, 1)
	sub, err := nc.Subscribe(
		"resystems.renotify.testuser.device.mb_TESTDEV00001.control",
		func(msg *nats.Msg) { msgCh <- msg })
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Unsubscribe()

	stdout, _, err := executeCommand("silent",
		"--device", "mb_TESTDEV00001", "on")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "on") {
		t.Errorf("stdout = %q, want 'on'", stdout)
	}

	nc.Flush()

	select {
	case msg := <-msgCh:
		var ctrl deviceControl
		if err := json.Unmarshal(msg.Data, &ctrl); err != nil {
			t.Fatal(err)
		}
		if ctrl.Command != "set_silent" {
			t.Errorf("command = %q, want set_silent",
				ctrl.Command)
		}
		if !ctrl.Value {
			t.Error("value should be true for 'on'")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for control message")
	}
}

func TestSilent_Off(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	devicesPath := filepath.Join(stateDir, "renotify",
		"pairing", "devices.json")
	os.MkdirAll(filepath.Dir(devicesPath), 0o755)
	state.AddDevice(devicesPath, state.PairedDevice{
		DeviceID: "mb_TESTDEV00001",
		Token:    "rn_tk_test",
		PairedAt: time.Now().UTC(),
	})

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	msgCh := make(chan *nats.Msg, 1)
	nc.Subscribe(
		"resystems.renotify.testuser.device.mb_TESTDEV00001.control",
		func(msg *nats.Msg) { msgCh <- msg })

	executeCommand("silent",
		"--device", "mb_TESTDEV00001", "off")

	nc.Flush()

	select {
	case msg := <-msgCh:
		var ctrl deviceControl
		json.Unmarshal(msg.Data, &ctrl)
		if ctrl.Value {
			t.Error("value should be false for 'off'")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for control message")
	}
}

func TestSilent_All(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	devicesPath := filepath.Join(stateDir, "renotify",
		"pairing", "devices.json")
	os.MkdirAll(filepath.Dir(devicesPath), 0o755)
	for _, id := range []string{
		"mb_TESTDEV00001", "mb_TESTDEV00002",
	} {
		state.AddDevice(devicesPath, state.PairedDevice{
			DeviceID: id,
			Token:    "rn_tk_test",
			PairedAt: time.Now().UTC(),
		})
	}

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	var received []*nats.Msg
	nc.Subscribe(
		"resystems.renotify.testuser.device.>",
		func(msg *nats.Msg) { received = append(received, msg) })

	stdout, _, err := executeCommand("silent", "--all", "on")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nc.Flush()
	time.Sleep(100 * time.Millisecond)

	if len(received) != 2 {
		t.Fatalf("got %d messages, want 2 (one per device)",
			len(received))
	}
	if !strings.Contains(stdout, "mb_TESTDEV00001") ||
		!strings.Contains(stdout, "mb_TESTDEV00002") {
		t.Errorf("stdout should mention both devices: %q",
			stdout)
	}
}
