// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

//go:build integration

package pairing

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.resystems.io/renotify/cli/internal/netutil"
)

func integrationConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		CertPath:     filepath.Join(dir, "tls", "cert.pem"),
		KeyPath:      filepath.Join(dir, "tls", "key.pem"),
		TokenPath:    filepath.Join(dir, "pairing", "token"),
		UsernamePath: filepath.Join(dir, "pairing", "username"),
		DevicesPath:  filepath.Join(dir, "pairing", "devices.json"),
		DaemonIDPath: filepath.Join(dir, "daemon_id"),
		Username:     "integrationuser",
		WSSPort:      4223,
		DiscoverIPs:  netutil.DiscoverIPs,
	}
}

func TestPair_EndToEnd_Text(t *testing.T) {
	cfg := integrationConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	// Verify result has all fields populated.
	if result.Host == "" {
		t.Error("empty host")
	}
	if result.Port != 4223 {
		t.Errorf("port = %d, want 4223", result.Port)
	}
	if !strings.HasPrefix(result.Token, "rn_tk_") {
		t.Errorf("token missing prefix: %q", result.Token)
	}
	if len(result.CertFingerprint) != 64 {
		t.Errorf("fingerprint length = %d, want 64",
			len(result.CertFingerprint))
	}
	if result.PayloadJSON == "" {
		t.Error("empty payload JSON")
	}
}

func TestPair_EndToEnd_JSON(t *testing.T) {
	cfg := integrationConfig(t)
	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	var payload ProvisioningPayload
	if err := json.Unmarshal([]byte(result.PayloadJSON), &payload); err != nil {
		t.Fatalf("invalid payload JSON: %v", err)
	}
	if payload.Version != 2 {
		t.Errorf("version = %d, want 2", payload.Version)
	}
	if payload.Host == "" {
		t.Error("empty host in payload")
	}
	if payload.Port != 4223 {
		t.Errorf("port = %d, want 4223", payload.Port)
	}
	if !strings.HasPrefix(payload.Token, "rn_tk_") {
		t.Errorf("token missing prefix: %q", payload.Token)
	}
	if len(payload.CertSHA) != 64 {
		t.Errorf("cert_sha length = %d, want 64", len(payload.CertSHA))
	}
}

func TestPair_EndToEnd_Idempotent(t *testing.T) {
	cfg := integrationConfig(t)

	r1, err := Pair(cfg)
	if err != nil {
		t.Fatalf("first Pair: %v", err)
	}

	r2, err := Pair(cfg)
	if err != nil {
		t.Fatalf("second Pair: %v", err)
	}

	// Same cert fingerprint (reused).
	if r2.CertFingerprint != r1.CertFingerprint {
		t.Errorf("fingerprint changed: %s -> %s",
			r1.CertFingerprint, r2.CertFingerprint)
	}
	// Different token (always new).
	if r2.Token == r1.Token {
		t.Error("token should change on second pair")
	}
}

func TestPair_EndToEnd_RegenerateCert(t *testing.T) {
	cfg := integrationConfig(t)

	r1, err := Pair(cfg)
	if err != nil {
		t.Fatalf("first Pair: %v", err)
	}

	cfg.RegenerateCert = true
	r2, err := Pair(cfg)
	if err != nil {
		t.Fatalf("second Pair: %v", err)
	}

	if r2.CertFingerprint == r1.CertFingerprint {
		t.Error("fingerprint should change after regeneration")
	}
	if !r2.CertRegenerated {
		t.Error("expected CertRegenerated=true")
	}
}

func TestPair_EndToEnd_IPOverride(t *testing.T) {
	cfg := integrationConfig(t)
	cfg.OverrideIP = "10.99.99.1"

	result, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if result.Host != "10.99.99.1" {
		t.Errorf("Host = %q, want 10.99.99.1", result.Host)
	}

	var payload ProvisioningPayload
	json.Unmarshal([]byte(result.PayloadJSON), &payload)
	if payload.Host != "10.99.99.1" {
		t.Errorf("payload host = %q, want 10.99.99.1", payload.Host)
	}
}

func TestPair_EndToEnd_CertSANs(t *testing.T) {
	cfg := integrationConfig(t)

	_, err := Pair(cfg)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	// Parse the generated certificate.
	certData, err := os.ReadFile(cfg.CertPath)
	if err != nil {
		t.Fatalf("read cert: %v", err)
	}
	block, _ := pem.Decode(certData)
	if block == nil {
		t.Fatal("no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}

	// Must include 127.0.0.1.
	found := false
	for _, ip := range cert.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			found = true
			break
		}
	}
	if !found {
		t.Error("cert SANs missing 127.0.0.1")
	}

	// Must include localhost.
	foundDNS := false
	for _, name := range cert.DNSNames {
		if name == "localhost" {
			foundDNS = true
			break
		}
	}
	if !foundDNS {
		t.Error("cert SANs missing localhost")
	}
}
