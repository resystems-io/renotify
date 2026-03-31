package config

import "fmt"

// ParamInfo describes a single configurable parameter. The
// Registry slice is the authoritative list of all parameters —
// setDefaults() iterates it to register keys with Viper, and
// the config init/list commands use it for output.
type ParamInfo struct {
	Key         string            // dot-path, e.g. "broker.tcp_port"
	Type        string            // display label: "int", "bool", "duration", etc.
	EnvVar      string            // e.g. "RENOTIFY_BROKER_TCP_PORT"
	Description string            // short description
	Resolve     func(*Config) any // extracts the typed value from a Config
}

// FormatDefault returns the default value as a display string,
// using the Duration string representation where appropriate.
func (p ParamInfo) FormatDefault(cfg *Config) string {
	val := p.Resolve(cfg)
	if d, ok := val.(Duration); ok {
		return d.Duration.String()
	}
	return fmt.Sprint(val)
}

// Registry is the single source of truth for all configurable
// parameter metadata. Default values come from Default() via
// each entry's Resolve function.
//
// See docs/analysis-configuration-schema.md for the full
// parameter catalogue.
var Registry = []ParamInfo{
	// --- Identity ---
	{
		Key: "username", Type: "string",
		EnvVar:      "RENOTIFY_USERNAME",
		Description: "NATS auth identity (default: system username)",
		Resolve:     func(c *Config) any { return c.Username },
	},

	// --- Embedded broker ---
	{
		Key: "broker.enabled", Type: "bool",
		EnvVar:      "RENOTIFY_BROKER_ENABLED",
		Description: "enable embedded NATS broker",
		Resolve:     func(c *Config) any { return c.Broker.Enabled },
	},
	{
		Key: "broker.tcp_host", Type: "string",
		EnvVar:      "RENOTIFY_BROKER_TCP_HOST",
		Description: "TCP listener bind address",
		Resolve:     func(c *Config) any { return c.Broker.TCPHost },
	},
	{
		Key: "broker.tcp_port", Type: "int",
		EnvVar:      "RENOTIFY_BROKER_TCP_PORT",
		Description: "TCP listener port",
		Resolve:     func(c *Config) any { return c.Broker.TCPPort },
	},
	{
		Key: "broker.wss_host", Type: "string",
		EnvVar:      "RENOTIFY_BROKER_WSS_HOST",
		Description: "WSS listener bind address",
		Resolve:     func(c *Config) any { return c.Broker.WSSHost },
	},
	{
		Key: "broker.wss_port", Type: "int",
		EnvVar:      "RENOTIFY_BROKER_WSS_PORT",
		Description: "WSS listener port (mobile app)",
		Resolve:     func(c *Config) any { return c.Broker.WSSPort },
	},
	{
		Key: "broker.cert_file", Type: "filepath",
		EnvVar:      "RENOTIFY_BROKER_CERT_FILE",
		Description: "TLS certificate path (PEM)",
		Resolve:     func(c *Config) any { return c.Broker.CertFile },
	},
	{
		Key: "broker.key_file", Type: "filepath",
		EnvVar:      "RENOTIFY_BROKER_KEY_FILE",
		Description: "TLS private key path (PEM)",
		Resolve:     func(c *Config) any { return c.Broker.KeyFile },
	},

	// --- MCP server ---
	{
		Key: "mcp.enabled", Type: "bool",
		EnvVar:      "RENOTIFY_MCP_ENABLED",
		Description: "enable MCP server",
		Resolve:     func(c *Config) any { return c.MCP.Enabled },
	},
	{
		Key: "mcp.host", Type: "string",
		EnvVar:      "RENOTIFY_MCP_HOST",
		Description: "MCP/HTTP listener bind address",
		Resolve:     func(c *Config) any { return c.MCP.Host },
	},
	{
		Key: "mcp.port", Type: "int",
		EnvVar:      "RENOTIFY_MCP_PORT",
		Description: "MCP/HTTP listener port",
		Resolve:     func(c *Config) any { return c.MCP.Port },
	},

	// --- JetStream ---
	{
		Key: "jetstream.max_age", Type: "duration",
		EnvVar:      "RENOTIFY_JETSTREAM_MAX_AGE",
		Description: "message TTL",
		Resolve:     func(c *Config) any { return c.JetStream.MaxAge },
	},
	{
		Key: "jetstream.max_bytes", Type: "int64",
		EnvVar:      "RENOTIFY_JETSTREAM_MAX_BYTES",
		Description: "total stream memory cap (bytes)",
		Resolve:     func(c *Config) any { return c.JetStream.MaxBytes },
	},
	{
		Key: "jetstream.max_msg_size", Type: "int32",
		EnvVar:      "RENOTIFY_JETSTREAM_MAX_MSG_SIZE",
		Description: "per-message size limit (bytes)",
		Resolve:     func(c *Config) any { return c.JetStream.MaxMsgSize },
	},
	{
		Key: "jetstream.max_msgs_per_subj", Type: "int64",
		EnvVar:      "RENOTIFY_JETSTREAM_MAX_MSGS_PER_SUBJ",
		Description: "max messages per subject",
		Resolve:     func(c *Config) any { return c.JetStream.MaxMsgsPerSubj },
	},
	{
		Key: "jetstream.dup_window", Type: "duration",
		EnvVar:      "RENOTIFY_JETSTREAM_DUP_WINDOW",
		Description: "deduplication window",
		Resolve:     func(c *Config) any { return c.JetStream.DupWindow },
	},

	// --- Shared broker ---
	{
		Key: "shared_broker.url", Type: "string",
		EnvVar:      "RENOTIFY_SHARED_BROKER_URL",
		Description: "external NATS broker URL",
		Resolve:     func(c *Config) any { return c.SharedBroker.URL },
	},
	{
		Key: "shared_broker.tls_enabled", Type: "bool",
		EnvVar:      "RENOTIFY_SHARED_BROKER_TLS_ENABLED",
		Description: "enable TLS for shared broker",
		Resolve:     func(c *Config) any { return c.SharedBroker.TLSEnabled },
	},

	// --- Operational ---
	{
		Key: "rate_limit.notifications_per_minute", Type: "int",
		EnvVar:      "RENOTIFY_RATE_LIMIT_NOTIFICATIONS_PER_MINUTE",
		Description: "per-flow notification rate limit",
		Resolve:     func(c *Config) any { return c.RateLimit.NotificationsPerMinute },
	},
	{
		Key: "reaping.grace_period", Type: "duration",
		EnvVar:      "RENOTIFY_REAPING_GRACE_PERIOD",
		Description: "inactivity before stale flow is reaped",
		Resolve:     func(c *Config) any { return c.Reaping.GracePeriod },
	},
	{
		Key: "timeout.default_ask_timeout", Type: "duration",
		EnvVar:      "RENOTIFY_TIMEOUT_DEFAULT_ASK_TIMEOUT",
		Description: "default blocking ask timeout",
		Resolve:     func(c *Config) any { return c.Timeout.DefaultAskTimeout },
	},
	{
		Key: "heartbeat.interval", Type: "duration",
		EnvVar:      "RENOTIFY_HEARTBEAT_INTERVAL",
		Description: "daemon heartbeat publish interval",
		Resolve:     func(c *Config) any { return c.Heartbeat.Interval },
	},
	{
		Key: "interjection.debounce_window", Type: "duration",
		EnvVar:      "RENOTIFY_INTERJECTION_DEBOUNCE_WINDOW",
		Description: "interjection dedup window",
		Resolve:     func(c *Config) any { return c.Interjection.DebounceWindow },
	},

	// --- Daemon runtime ---
	{
		Key: "daemon.foreground", Type: "bool",
		EnvVar:      "RENOTIFY_DAEMON_FOREGROUND",
		Description: "run daemon in foreground",
		Resolve:     func(c *Config) any { return c.Daemon.Foreground },
	},
	{
		Key: "daemon.log_level", Type: "string",
		EnvVar:      "RENOTIFY_DAEMON_LOG_LEVEL",
		Description: "log level: debug, info, warn, error",
		Resolve:     func(c *Config) any { return c.Daemon.LogLevel },
	},
	{
		Key: "daemon.log_file", Type: "filepath",
		EnvVar:      "RENOTIFY_DAEMON_LOG_FILE",
		Description: "log file path (background mode)",
		Resolve:     func(c *Config) any { return c.Daemon.LogFile },
	},
	{
		Key: "daemon.db_path", Type: "filepath",
		EnvVar:      "RENOTIFY_DAEMON_DB_PATH",
		Description: "SQLite ledger database path",
		Resolve:     func(c *Config) any { return c.Daemon.DBPath },
	},
}
