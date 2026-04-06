package broker

import (
	"log/slog"
	"net"
	"path/filepath"
	"testing"

	"go.resystems.io/renotify/cli/internal/state"
	"go.resystems.io/renotify/cli/internal/tlsgen"
)

func TestNewEmbeddedServer_TCPOnlyOptions(t *testing.T) {
	srv, err := NewEmbeddedServer(EmbeddedConfig{
		TCPHost:         "127.0.0.1",
		TCPPort:         -1,
		Username:        "testuser",
		InternalToken:   "tok",
		JetStreamMaxMem: 64 * 1024 * 1024,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if srv.opts.Websocket.Port != 0 {
		t.Error("WSS should not be configured without TLS")
	}
	if srv.opts.TLSCert != "" {
		t.Error("TLS cert should be empty")
	}
}

func TestNewEmbeddedServer_TCPAndWSSOptions(t *testing.T) {
	// Generate a real cert/key pair for the test.
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	result, err := tlsgen.Generate(tlsgen.GenerateParams{
		DaemonID:  "dn_test",
		IPs:       []net.IP{net.ParseIP("127.0.0.1")},
		Hostnames: []string{"localhost"},
	})
	if err != nil {
		t.Fatal(err)
	}
	state.WriteTLS(certPath, result.CertPEM, keyPath, result.KeyPEM)

	srv, err := NewEmbeddedServer(EmbeddedConfig{
		TCPHost:         "127.0.0.1",
		TCPPort:         -1,
		WSSHost:         "0.0.0.0",
		WSSPort:         -1,
		TLSCert:         certPath,
		TLSKey:          keyPath,
		Username:        "testuser",
		InternalToken:   "tok",
		JetStreamMaxMem: 64 * 1024 * 1024,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if srv.opts.Websocket.Host != "0.0.0.0" {
		t.Errorf("WSS host = %q, want %q", srv.opts.Websocket.Host, "0.0.0.0")
	}
	if srv.opts.Websocket.TLSConfig == nil {
		t.Error("Websocket TLSConfig should be set")
	}
}

func TestNewEmbeddedServer_JetStreamEnabled(t *testing.T) {
	srv, err := NewEmbeddedServer(EmbeddedConfig{
		TCPHost:         "127.0.0.1",
		TCPPort:         -1,
		Username:        "testuser",
		InternalToken:   "tok",
		JetStreamMaxMem: 128 * 1024 * 1024,
	}, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if !srv.opts.JetStream {
		t.Error("JetStream should be enabled")
	}
	if srv.opts.JetStreamMaxMemory != 128*1024*1024 {
		t.Errorf("JetStream max mem = %d, want %d",
			srv.opts.JetStreamMaxMemory, 128*1024*1024)
	}
	if srv.opts.StoreDir == "" {
		t.Error("StoreDir should be a temp dir for JetStream metadata")
	}
}
