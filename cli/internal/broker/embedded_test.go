package broker

import (
	"log/slog"
	"testing"
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
	srv, err := NewEmbeddedServer(EmbeddedConfig{
		TCPHost:         "127.0.0.1",
		TCPPort:         -1,
		WSSHost:         "0.0.0.0",
		WSSPort:         -1,
		TLSCert:         "/tmp/cert.pem",
		TLSKey:          "/tmp/key.pem",
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
	if srv.opts.TLSCert != "/tmp/cert.pem" {
		t.Errorf("TLS cert = %q, want %q", srv.opts.TLSCert, "/tmp/cert.pem")
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
