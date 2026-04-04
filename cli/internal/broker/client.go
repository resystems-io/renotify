package broker

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/xdg"
)

// ConnectEmbedded connects a NATS client to the embedded server
// using an in-process pipe (R-CLI-22, C-19). The daemon bypasses
// the TCP listener entirely; CLI processes still connect via TCP.
func ConnectEmbedded(srv *server.Server, token string, logger *slog.Logger) (*nats.Conn, error) {
	nc, err := nats.Connect(srv.ClientURL(),
		nats.InProcessServer(srv),
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

// ConnectCLI connects a short-lived CLI command (post, ask,
// history) to the daemon's NATS broker. It reads the config to
// determine embedded vs shared mode and loads the appropriate
// credentials. The connection does not reconnect — CLI commands
// are single-shot.
func ConnectCLI(cfg *config.Config) (*nats.Conn, error) {
	if cfg.Broker.Enabled {
		return connectCLIEmbedded(cfg)
	}
	return connectCLIShared(cfg)
}

func connectCLIEmbedded(cfg *config.Config) (*nats.Conn, error) {
	data, err := os.ReadFile(xdg.InternalTokenPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"daemon has not been started " +
					"(internal token not found)")
		}
		return nil, fmt.Errorf("read internal token: %w", err)
	}
	token := strings.TrimSpace(string(data))

	url := fmt.Sprintf("nats://%s:%d",
		cfg.Broker.TCPHost, cfg.Broker.TCPPort)

	nc, err := nats.Connect(url,
		nats.UserInfo("daemon", token),
		nats.Name("renotify-cli"),
		nats.MaxReconnects(0),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to daemon: %w", err)
	}
	return nc, nil
}

func connectCLIShared(cfg *config.Config) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("renotify-cli"),
		nats.MaxReconnects(0),
	}

	if cfg.SharedBroker.Token != "" {
		opts = append(opts, nats.Token(cfg.SharedBroker.Token))
	} else if cfg.SharedBroker.Username != "" {
		opts = append(opts,
			nats.UserInfo(cfg.SharedBroker.Username,
				cfg.SharedBroker.Password))
	}

	if cfg.SharedBroker.TLSEnabled {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if cfg.SharedBroker.CACert != "" {
			opts = append(opts, nats.RootCAs(cfg.SharedBroker.CACert))
		}
		if cfg.SharedBroker.ClientCert != "" &&
			cfg.SharedBroker.ClientKey != "" {
			cert, err := tls.LoadX509KeyPair(
				cfg.SharedBroker.ClientCert,
				cfg.SharedBroker.ClientKey)
			if err != nil {
				return nil, fmt.Errorf("load client cert: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		opts = append(opts, nats.Secure(tlsCfg))
	}

	nc, err := nats.Connect(cfg.SharedBroker.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("connect to shared broker: %w", err)
	}
	return nc, nil
}
