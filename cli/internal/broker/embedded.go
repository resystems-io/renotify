// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package broker

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"

	"go.resystems.io/renotify/cli/internal/state"
)

// readyTimeout is how long Start waits for the embedded NATS
// server to accept connections before returning an error.
const readyTimeout = 10 * time.Second

// EmbeddedConfig holds parameters for the in-process NATS server.
type EmbeddedConfig struct {
	TCPHost  string
	TCPPort  int
	WSSHost  string
	WSSPort  int
	TLSCert  string // empty → skip WSS listener
	TLSKey   string
	Username string

	InternalToken string
	Devices       []state.PairedDevice // paired mobile devices

	JetStreamMaxMem int64
	StoreDir        string // persistent store directory path (if non-empty)
}

// EmbeddedServer wraps a nats-server with Renotify configuration.
type EmbeddedServer struct {
	srv       *server.Server
	opts      *server.Options
	logger    *slog.Logger
	storeDir  string // dir for JetStream metadata
	isTempDir bool   // whether storeDir is temporary and needs cleanup
}

// NewEmbeddedServer creates an embedded NATS server from the
// given configuration. Call Start() to begin listening.
func NewEmbeddedServer(cfg EmbeddedConfig, logger *slog.Logger) (*EmbeddedServer, error) {
	var storeDir string
	var isTemp bool
	var err error

	if cfg.StoreDir != "" {
		storeDir = cfg.StoreDir
	} else {
		// JetStream needs a StoreDir even for memory-only storage
		// (metadata). Use a unique temp dir to avoid conflicts
		// between concurrent instances.
		storeDir, err = os.MkdirTemp("", "renotify-jetstream-*")
		if err != nil {
			return nil, fmt.Errorf("create JetStream store dir: %w", err)
		}
		isTemp = true
	}

	opts := &server.Options{
		ServerName:         fmt.Sprintf("renotify-%d", os.Getpid()),
		Host:               cfg.TCPHost,
		Port:               cfg.TCPPort,
		JetStream:          true,
		JetStreamMaxMemory: cfg.JetStreamMaxMem,
		StoreDir:           storeDir,
		NoLog:              true,
		NoSigs:             true,
		PingInterval:       20 * time.Second,
		MaxPingsOut:        2,
		Users:              BuildAuthConfig(cfg.Username, cfg.InternalToken, cfg.Devices),
	}

	// Configure WSS listener if TLS is available. Load the
	// certificate directly into the Websocket TLSConfig rather
	// than using TLSMap (which requires server-level TLS on the
	// TCP listener, which we don't want for loopback-only TCP).
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
		if err != nil {
			if isTemp {
				os.RemoveAll(storeDir)
			}
			return nil, fmt.Errorf("load TLS cert/key: %w", err)
		}
		opts.Websocket = server.WebsocketOpts{
			Host:  cfg.WSSHost,
			Port:  cfg.WSSPort,
			NoTLS: false,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			},
		}
	}

	return &EmbeddedServer{
		opts:      opts,
		logger:    logger,
		storeDir:  storeDir,
		isTempDir: isTemp,
	}, nil
}

// Start creates and starts the embedded NATS server. It blocks
// until the server is ready for connections or returns an error.
func (s *EmbeddedServer) Start() error {
	srv, err := server.NewServer(s.opts)
	if err != nil {
		return fmt.Errorf("create NATS server: %w", err)
	}
	s.srv = srv
	s.srv.Start()

	if !s.srv.ReadyForConnections(readyTimeout) {
		s.srv.Shutdown()
		return fmt.Errorf("NATS server failed to become ready")
	}

	s.logger.Info("embedded NATS server started",
		"tcp", s.srv.Addr().String(),
		"jetstream", true,
	)
	return nil
}

// Shutdown gracefully stops the embedded NATS server and cleans
// up the JetStream metadata directory.
func (s *EmbeddedServer) Shutdown(_ context.Context) error {
	if s.srv != nil {
		s.srv.Shutdown()
		s.srv.WaitForShutdown()
		s.logger.Info("embedded NATS server stopped")
	}
	if s.storeDir != "" && s.isTempDir {
		os.RemoveAll(s.storeDir)
	}
	return nil
}

// ClientURL returns the TCP client URL for the embedded server.
func (s *EmbeddedServer) ClientURL() string {
	if s.srv != nil {
		return s.srv.ClientURL()
	}
	return fmt.Sprintf("nats://%s:%d", s.opts.Host, s.opts.Port)
}

// ReloadAuth rebuilds the NATS auth configuration with the
// given devices and applies it to the running server via
// ReloadOptions. Called on SIGHUP after `renotify pair` or
// `renotify revoke` updates devices.json.
func (s *EmbeddedServer) ReloadAuth(
	username, internalToken string,
	devices []state.PairedDevice,
) error {
	if s.srv == nil {
		return fmt.Errorf("server not started")
	}
	newOpts := *s.opts
	newOpts.Users = BuildAuthConfig(username, internalToken, devices)
	if err := s.srv.ReloadOptions(&newOpts); err != nil {
		return fmt.Errorf("reload auth: %w", err)
	}
	return nil
}

// Server returns the underlying nats-server instance for testing.
func (s *EmbeddedServer) Server() *server.Server {
	return s.srv
}
