package pairing

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.resystems.io/renotify/internal/state"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		CertPath:     filepath.Join(dir, "tls", "cert.pem"),
		KeyPath:      filepath.Join(dir, "tls", "key.pem"),
		TokenPath:    filepath.Join(dir, "pairing", "token"),
		UsernamePath: filepath.Join(dir, "pairing", "username"),
		DevicesPath:  filepath.Join(dir, "pairing", "devices.json"),
		DaemonIDPath: filepath.Join(dir, "daemon_id"),
		Username:     "testuser",
		WSSPort:      4223,
		DiscoverIPs: func() ([]net.IP, error) {
			return []net.IP{net.ParseIP("192.168.1.42")}, nil
		},
	}
}

func TestPair_FirstRun(t *testing.T) {
	cfg := testConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	if result.Token == "" {
		t.Error("expected non-empty token")
	}
	if result.CertFingerprint == "" {
		t.Error("expected non-empty fingerprint")
	}
	if !result.CertRegenerated {
		t.Error("expected CertRegenerated=true on first run")
	}
	if result.Host != "192.168.1.42" {
		t.Errorf("Host = %q, want 192.168.1.42", result.Host)
	}
	if result.Port != 4223 {
		t.Errorf("Port = %d, want 4223", result.Port)
	}

	if result.DeviceID == "" {
		t.Error("expected non-empty device_id")
	}
	if !strings.HasPrefix(result.DeviceID, "mb_") {
		t.Errorf("device_id = %q, want mb_ prefix", result.DeviceID)
	}

	// Verify files were created.
	if _, err := os.Stat(cfg.CertPath); err != nil {
		t.Errorf("cert file not created: %v", err)
	}
	if _, err := os.Stat(cfg.KeyPath); err != nil {
		t.Errorf("key file not created: %v", err)
	}
	if _, err := os.Stat(cfg.DevicesPath); err != nil {
		t.Errorf("devices.json not created: %v", err)
	}
}

func TestPair_ExistingCert(t *testing.T) {
	cfg := testConfig(t)

	// First pair: generates cert.
	r1, err := Pair(cfg)
	if err != nil {
		t.Fatalf("first Pair: %v", err)
	}

	// Second pair: reuses cert.
	r2, err := Pair(cfg)
	if err != nil {
		t.Fatalf("second Pair: %v", err)
	}

	if r2.CertRegenerated {
		t.Error("expected CertRegenerated=false on second run")
	}
	if r2.CertFingerprint != r1.CertFingerprint {
		t.Errorf("fingerprint changed: %s -> %s",
			r1.CertFingerprint, r2.CertFingerprint)
	}
}

func TestPair_RegenerateCert(t *testing.T) {
	cfg := testConfig(t)

	r1, err := Pair(cfg)
	if err != nil {
		t.Fatalf("first Pair: %v", err)
	}

	cfg.RegenerateCert = true
	r2, err := Pair(cfg)
	if err != nil {
		t.Fatalf("second Pair: %v", err)
	}

	if !r2.CertRegenerated {
		t.Error("expected CertRegenerated=true")
	}
	if r2.CertFingerprint == r1.CertFingerprint {
		t.Error("fingerprint should change after regeneration")
	}
}

func TestPair_OverrideIP(t *testing.T) {
	cfg := testConfig(t)
	cfg.OverrideIP = "10.99.99.1"

	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if result.Host != "10.99.99.1" {
		t.Errorf("Host = %q, want 10.99.99.1", result.Host)
	}
}

func TestPair_DiscoveredIP(t *testing.T) {
	cfg := testConfig(t)
	cfg.DiscoverIPs = func() ([]net.IP, error) {
		return []net.IP{
			net.ParseIP("172.16.0.5"),
			net.ParseIP("192.168.1.42"),
		}, nil
	}

	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	// PreferredIP selects first IPv4 private.
	if result.Host != "172.16.0.5" {
		t.Errorf("Host = %q, want 172.16.0.5", result.Host)
	}
}

func TestPair_EachPairAddsDevice(t *testing.T) {
	cfg := testConfig(t)

	r1, err := Pair(cfg)
	if err != nil {
		t.Fatalf("first Pair: %v", err)
	}

	r2, err := Pair(cfg)
	if err != nil {
		t.Fatalf("second Pair: %v", err)
	}

	if r2.Token == r1.Token {
		t.Error("second pair should generate a new token")
	}
	if r2.DeviceID == r1.DeviceID {
		t.Error("second pair should generate a new device_id")
	}

	devices, _ := state.LoadDevices(cfg.DevicesPath)
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}
}

func TestPair_PayloadJSON(t *testing.T) {
	cfg := testConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	var payload ProvisioningPayload
	if err := json.Unmarshal([]byte(result.PayloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload.Host != result.Host {
		t.Errorf("payload host = %q, want %q", payload.Host, result.Host)
	}
	if payload.Port != result.Port {
		t.Errorf("payload port = %d, want %d", payload.Port, result.Port)
	}
	if payload.Token != result.Token {
		t.Errorf("payload token mismatch")
	}
	if payload.CertSHA != result.CertFingerprint {
		t.Errorf("payload cert_sha mismatch")
	}
}

func TestPair_PayloadVersion(t *testing.T) {
	cfg := testConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	var payload ProvisioningPayload
	json.Unmarshal([]byte(result.PayloadJSON), &payload)
	if payload.Version != 2 {
		t.Errorf("version = %d, want 2", payload.Version)
	}
}

func TestPair_PayloadMinifiedKeys(t *testing.T) {
	cfg := testConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	// Verify single-character JSON keys.
	for _, key := range []string{`"v":`, `"h":`, `"p":`, `"t":`, `"c":`, `"u":`, `"d":`, `"n":`} {
		if !strings.Contains(result.PayloadJSON, key) {
			t.Errorf("payload missing key %s", key)
		}
	}
}

func TestPair_PayloadUsername(t *testing.T) {
	cfg := testConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	var payload ProvisioningPayload
	json.Unmarshal([]byte(result.PayloadJSON), &payload)
	if payload.Username != "testuser" {
		t.Errorf("username = %q, want testuser", payload.Username)
	}
}

func TestPair_DeviceAddedToRegistry(t *testing.T) {
	cfg := testConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	devices, err := state.LoadDevices(cfg.DevicesPath)
	if err != nil {
		t.Fatalf("load devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if devices[0].DeviceID != result.DeviceID {
		t.Errorf("device_id = %q, want %q",
			devices[0].DeviceID, result.DeviceID)
	}
	if devices[0].Token != result.Token {
		t.Errorf("token mismatch")
	}
}

func TestPair_DaemonIDCreated(t *testing.T) {
	cfg := testConfig(t)
	_, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	data, err := os.ReadFile(cfg.DaemonIDPath)
	if err != nil {
		t.Fatalf("read daemon_id: %v", err)
	}
	id := strings.TrimSpace(string(data))
	if !strings.HasPrefix(id, "dn_") {
		t.Errorf("daemon_id = %q, expected dn_ prefix", id)
	}
}

func TestPair_DiscoveryError(t *testing.T) {
	cfg := testConfig(t)
	cfg.DiscoverIPs = func() ([]net.IP, error) {
		return nil, net.UnknownNetworkError("test error")
	}

	_, err := Pair(cfg)
	if err == nil {
		t.Fatal("expected error from failing DiscoverIPs")
	}
	if !strings.Contains(err.Error(), "discover IPs") {
		t.Errorf("error = %q, expected 'discover IPs'", err)
	}
}
