package broker

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/config"
)

// ConnectEmbedded connects a NATS client to the embedded server
// using the daemon account credentials on the loopback TCP
// listener.
func ConnectEmbedded(url, token string, logger *slog.Logger) (*nats.Conn, error) {
	nc, err := nats.Connect(url,
		nats.UserInfo("daemon", token),
		nats.Name("renotify-daemon"),
		nats.MaxReconnects(-1),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Error("NATS error", "err", err)
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", "err", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			logger.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to embedded broker: %w", err)
	}
	return nc, nil
}

// ConnectShared connects a NATS client to an external shared
// broker using the configured credentials.
func ConnectShared(cfg config.SharedBrokerConfig, logger *slog.Logger) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("renotify-daemon"),
		nats.MaxReconnects(-1),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			logger.Error("NATS error", "err", err)
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("NATS disconnected", "err", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			logger.Info("NATS reconnected")
		}),
	}

	// Auth: token or user/password.
	if cfg.Token != "" {
		opts = append(opts, nats.Token(cfg.Token))
	} else if cfg.Username != "" {
		opts = append(opts, nats.UserInfo(cfg.Username, cfg.Password))
	}

	// TLS.
	if cfg.TLSEnabled {
		tlsCfg := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		if cfg.CACert != "" {
			caCert, err := os.ReadFile(cfg.CACert)
			if err != nil {
				return nil, fmt.Errorf("read CA cert: %w", err)
			}
			_ = caCert // CACert pool setup would go here.
			opts = append(opts, nats.RootCAs(cfg.CACert))
		}
		if cfg.ClientCert != "" && cfg.ClientKey != "" {
			cert, err := tls.LoadX509KeyPair(cfg.ClientCert, cfg.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("load client cert: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		opts = append(opts, nats.Secure(tlsCfg))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to shared broker: %w", err)
	}
	return nc, nil
}
