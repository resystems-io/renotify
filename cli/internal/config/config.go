package config

import (
	"fmt"
	"os/user"
	"time"

	"go.resystems.io/renotify/internal/xdg"
)

// Config is the top-level daemon configuration. All fields have
// compiled defaults; the settings.json file provides overrides.
// See docs/analysis-configuration-schema.md Section 2.
type Config struct {
	Username     string             `json:"username"      mapstructure:"username"`
	Broker       BrokerConfig       `json:"broker"        mapstructure:"broker"`
	MCP          MCPConfig          `json:"mcp"           mapstructure:"mcp"`
	JetStream    JetStreamConfig    `json:"jetstream"     mapstructure:"jetstream"`
	SharedBroker SharedBrokerConfig `json:"shared_broker" mapstructure:"shared_broker"`
	RateLimit    RateLimitConfig    `json:"rate_limit"    mapstructure:"rate_limit"`
	Reaping      ReapingConfig      `json:"reaping"       mapstructure:"reaping"`
	Timeout      TimeoutConfig      `json:"timeout"       mapstructure:"timeout"`
	Heartbeat    HeartbeatConfig    `json:"heartbeat"     mapstructure:"heartbeat"`
	Interjection InterjectionConfig `json:"interjection"  mapstructure:"interjection"`
	Daemon       DaemonConfig       `json:"daemon"        mapstructure:"daemon"`
}

// BrokerConfig controls the embedded NATS server. When Enabled is
// false, the daemon connects to an external shared broker using
// the SharedBroker settings.
type BrokerConfig struct {
	Enabled  bool   `json:"enabled"   mapstructure:"enabled"`
	TCPHost  string `json:"tcp_host"  mapstructure:"tcp_host"`
	TCPPort  int    `json:"tcp_port"  mapstructure:"tcp_port"`
	WSSHost  string `json:"wss_host"  mapstructure:"wss_host"`
	WSSPort  int    `json:"wss_port"  mapstructure:"wss_port"`
	CertFile string `json:"cert_file" mapstructure:"cert_file"`
	KeyFile  string `json:"key_file"  mapstructure:"key_file"`
}

// MCPConfig controls the MCP server. The daemon runs an HTTP
// server on Host:Port serving MCP via SSE at /mcp. Multiple
// concurrent AI agent sessions connect to this shared endpoint.
type MCPConfig struct {
	Enabled bool   `json:"enabled" mapstructure:"enabled"`
	Host    string `json:"host"    mapstructure:"host"`
	Port    int    `json:"port"    mapstructure:"port"`
}

// JetStreamConfig holds the configurable parameters for the
// RENOTIFY JetStream stream.
type JetStreamConfig struct {
	MaxAge         Duration `json:"max_age"           mapstructure:"max_age"`
	MaxBytes       int64    `json:"max_bytes"         mapstructure:"max_bytes"`
	MaxMsgSize     int32    `json:"max_msg_size"      mapstructure:"max_msg_size"`
	MaxMsgsPerSubj int64    `json:"max_msgs_per_subj" mapstructure:"max_msgs_per_subj"`
	DupWindow      Duration `json:"dup_window"        mapstructure:"dup_window"`
}

// SharedBrokerConfig provides connection details for an external
// NATS broker. Used only when broker.enabled is false.
type SharedBrokerConfig struct {
	URL        string `json:"url"                    mapstructure:"url"`
	Username   string `json:"username,omitempty"      mapstructure:"username"`
	Password   string `json:"password,omitempty"      mapstructure:"password"`
	Token      string `json:"token,omitempty"         mapstructure:"token"`
	TLSEnabled bool   `json:"tls_enabled,omitempty"   mapstructure:"tls_enabled"`
	CACert     string `json:"ca_cert,omitempty"       mapstructure:"ca_cert"`
	ClientCert string `json:"client_cert,omitempty"   mapstructure:"client_cert"`
	ClientKey  string `json:"client_key,omitempty"    mapstructure:"client_key"`
}

// RateLimitConfig controls per-flow notification rate limiting.
type RateLimitConfig struct {
	NotificationsPerMinute int `json:"notifications_per_minute" mapstructure:"notifications_per_minute"`
}

// ReapingConfig controls stale flow detection.
type ReapingConfig struct {
	GracePeriod Duration `json:"grace_period" mapstructure:"grace_period"`
}

// TimeoutConfig controls blocking request timeouts.
type TimeoutConfig struct {
	DefaultAskTimeout Duration `json:"default_ask_timeout" mapstructure:"default_ask_timeout"`
	AskGracePeriod    Duration `json:"ask_grace_period"    mapstructure:"ask_grace_period"`
}

// HeartbeatConfig controls daemon heartbeat publishing.
type HeartbeatConfig struct {
	Interval Duration `json:"interval" mapstructure:"interval"`
}

// InterjectionConfig controls interjection processing.
// InterjectionCommand is the only non-idempotent payload in the
// system: a duplicate "stop" from a rapid double-tap would trigger
// redundant termination logic without debouncing. The stop handler
// includes a no-op guard as defence in depth.
type InterjectionConfig struct {
	DebounceWindow Duration `json:"debounce_window" mapstructure:"debounce_window"`
}

// DaemonConfig controls daemon process behaviour.
type DaemonConfig struct {
	Foreground bool   `json:"foreground" mapstructure:"foreground"`
	LogLevel   string `json:"log_level"  mapstructure:"log_level"`
	LogFile    string `json:"log_file"   mapstructure:"log_file"`
	DBPath     string `json:"db_path"    mapstructure:"db_path"`
}

// Default returns a Config with all compiled defaults applied.
func Default() *Config {
	return &Config{
		Username: defaultUsername(),
		Broker: BrokerConfig{
			Enabled:  true,
			TCPHost:  "127.0.0.1",
			TCPPort:  4222,
			WSSHost:  "0.0.0.0",
			WSSPort:  4223,
			CertFile: xdg.TLSCertPath(),
			KeyFile:  xdg.TLSKeyPath(),
		},
		MCP: MCPConfig{
			Enabled: true,
			Host:    "127.0.0.1",
			Port:    4224,
		},
		JetStream: JetStreamConfig{
			MaxAge:         NewDuration(30 * time.Minute),
			MaxBytes:       134217728, // 128 MB
			MaxMsgSize:     65536,     // 64 KB
			MaxMsgsPerSubj: 1000,
			DupWindow:      NewDuration(2 * time.Minute),
		},
		SharedBroker: SharedBrokerConfig{
			TLSEnabled: false,
		},
		RateLimit: RateLimitConfig{
			NotificationsPerMinute: 60,
		},
		Reaping: ReapingConfig{
			GracePeriod: NewDuration(5 * time.Minute),
		},
		Timeout: TimeoutConfig{
			DefaultAskTimeout: NewDuration(5 * time.Minute),
			AskGracePeriod:    NewDuration(30 * time.Second),
		},
		Heartbeat: HeartbeatConfig{
			Interval: NewDuration(30 * time.Second),
		},
		Interjection: InterjectionConfig{
			DebounceWindow: NewDuration(5 * time.Second),
		},
		Daemon: DaemonConfig{
			Foreground: false,
			LogLevel:   "info",
			LogFile:    xdg.DaemonLogPath(),
			DBPath:     xdg.DBPath(),
		},
	}
}

// Validate checks all configuration constraints. Returns the first
// validation error encountered, or nil if valid. See
// docs/analysis-configuration-schema.md Section 5.
func (c *Config) Validate() error {
	if c.Username == "" {
		return fmt.Errorf("username is required")
	}

	if c.Broker.TCPPort < 1 || c.Broker.TCPPort > 65535 {
		return fmt.Errorf("broker.tcp_port out of range: %d", c.Broker.TCPPort)
	}
	if c.Broker.WSSPort < 1 || c.Broker.WSSPort > 65535 {
		return fmt.Errorf("broker.wss_port out of range: %d", c.Broker.WSSPort)
	}
	if c.Broker.TCPPort == c.Broker.WSSPort {
		return fmt.Errorf("broker.wss_port must differ from broker.tcp_port")
	}

	if !c.Broker.Enabled && c.SharedBroker.URL == "" {
		return fmt.Errorf(
			"shared_broker.url is required when embedded broker is disabled",
		)
	}
	if c.SharedBroker.Username != "" && c.SharedBroker.Token != "" {
		return fmt.Errorf(
			"shared_broker: username/password and token are mutually exclusive",
		)
	}

	if c.MCP.Port < 1 || c.MCP.Port > 65535 {
		return fmt.Errorf("mcp.port out of range: %d", c.MCP.Port)
	}
	if c.MCP.Enabled && c.MCP.Port == c.Broker.TCPPort {
		return fmt.Errorf("mcp.port must differ from broker.tcp_port")
	}
	if c.MCP.Enabled && c.MCP.Port == c.Broker.WSSPort {
		return fmt.Errorf("mcp.port must differ from broker.wss_port")
	}

	if c.JetStream.MaxAge.Duration <= 0 {
		return fmt.Errorf("jetstream.max_age must be positive")
	}
	if c.JetStream.MaxBytes <= 0 {
		return fmt.Errorf("jetstream.max_bytes must be positive")
	}
	if c.JetStream.MaxMsgSize < 1 || c.JetStream.MaxMsgSize > 1048576 {
		return fmt.Errorf("jetstream.max_msg_size out of range: %d", c.JetStream.MaxMsgSize)
	}
	if c.JetStream.MaxMsgsPerSubj <= 0 {
		return fmt.Errorf("jetstream.max_msgs_per_subj must be positive")
	}
	if c.JetStream.DupWindow.Duration <= 0 {
		return fmt.Errorf("jetstream.dup_window must be positive")
	}
	if c.JetStream.DupWindow.Duration > c.JetStream.MaxAge.Duration {
		return fmt.Errorf("jetstream.dup_window must not exceed max_age")
	}

	if c.RateLimit.NotificationsPerMinute <= 0 {
		return fmt.Errorf("rate_limit.notifications_per_minute must be positive")
	}
	if c.Reaping.GracePeriod.Duration < time.Minute {
		return fmt.Errorf("reaping.grace_period must be at least 1 minute")
	}
	if c.Timeout.DefaultAskTimeout.Duration <= 0 {
		return fmt.Errorf("timeout.default_ask_timeout must be positive")
	}
	if c.Timeout.AskGracePeriod.Duration <= 0 {
		return fmt.Errorf("timeout.ask_grace_period must be positive")
	}
	if c.Heartbeat.Interval.Duration < 5*time.Second {
		return fmt.Errorf("heartbeat.interval must be at least 5 seconds")
	}
	if c.Interjection.DebounceWindow.Duration < time.Second {
		return fmt.Errorf("interjection.debounce_window must be at least 1 second")
	}

	switch c.Daemon.LogLevel {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return fmt.Errorf(
			"daemon.log_level must be debug, info, warn, or error: %q",
			c.Daemon.LogLevel)
	}

	return nil
}

// defaultUsername returns the current Unix username, or "" if it
// cannot be determined. The validation still requires a non-empty
// username — this just provides a sensible default.
func defaultUsername() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}
