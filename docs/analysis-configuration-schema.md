# Configuration Schema

This document defines the unified configuration schema for the
Renotify daemon and CLI. It consolidates all configurable parameters
into a single reference with Go struct definitions, JSON layout,
defaults, environment variable bindings, and validation rules.

For the NATS transport parameters that this configuration governs,
see the [NATS Transport Design](analysis-nats-transport-design.md).
For the SQLite database path and schema, see the
[SQLite Ledger Schema](analysis-sqlite-ledger.md).

This document addresses refinement item **A-06** (Configuration
Schema).

---

## 1. XDG Directory Layout

All file paths follow the [XDG Base Directory
Specification][xdg-basedir] (R-CLI-09). Two top-level directories
are used:

* **`$XDG_CONFIG_HOME/renotify/`** — user-editable configuration.
  Defaults to `~/.config/renotify/`.
* **`$XDG_STATE_HOME/renotify/`** — daemon-managed runtime state.
  Defaults to `~/.local/state/renotify/`.

```
$XDG_CONFIG_HOME/renotify/
└── settings.json              # User configuration (optional)

$XDG_STATE_HOME/renotify/
├── daemon_id                  # Auto-generated daemon identifier
├── internal_token             # Embedded broker internal auth token (0600)
├── renotify.db                # SQLite ledger (see analysis-sqlite-ledger.md)
├── daemon.log                 # Log file (background mode)
├── tls/
│   ├── cert.pem               # Self-signed ECDSA P-256 cert (0644)
│   └── key.pem                # TLS private key (0600)
└── pairing/
    ├── token                  # Mobile auth token (0600)
    └── username               # Associated username (0600)
```

### 1.1 Config vs State

| Directory | Owner | Editable by User | Backed Up | Purpose |
| :--- | :--- | :--- | :--- | :--- |
| `$XDG_CONFIG_HOME/renotify/` | User | Yes | Yes | Preferences and overrides |
| `$XDG_STATE_HOME/renotify/` | Daemon | No (managed by daemon/CLI commands) | Optional | Runtime identity, credentials, database, logs |

The `settings.json` file is the only user-editable artifact. All
files under `$XDG_STATE_HOME` are created and managed by the daemon
or by CLI commands (`renotify pair`, `renotify revoke`). Users
should not edit state files directly.

### 1.2 First-Run Behaviour

The daemon starts with compiled defaults if no `settings.json`
exists. It does not auto-create a configuration file. Users create
one only when they need to override defaults. This avoids
cluttering the filesystem and follows the principle that config
files are opt-in overrides, not required scaffolding.

State files are created on demand:
* `daemon_id` — generated on first daemon startup.
* `tls/cert.pem` and `tls/key.pem` — generated on first
  `renotify pair`.
* `pairing/token` and `pairing/username` — generated on each
  `renotify pair`.
* `renotify.db` — created on first daemon startup (schema V1
  migration applied automatically).
* `daemon.log` — created when the daemon runs in background mode.

---

## 2. Configuration Struct

The following Go struct defines the complete configuration schema.
Field names use `snake_case` JSON tags to match the `settings.json`
format. Viper binds to these paths using dot-separated keys (e.g.,
`broker.wss_port`).

```go
// Config is the top-level daemon configuration. All fields have
// compiled defaults; the settings.json file provides overrides.
type Config struct {
	Username     string             `json:"username"`
	Broker       BrokerConfig       `json:"broker"`
	MCP          MCPConfig          `json:"mcp"`
	JetStream    JetStreamConfig    `json:"jetstream"`
	SharedBroker SharedBrokerConfig `json:"shared_broker"`
	RateLimit    RateLimitConfig    `json:"rate_limit"`
	Reaping      ReapingConfig      `json:"reaping"`
	Timeout      TimeoutConfig      `json:"timeout"`
	Heartbeat    HeartbeatConfig    `json:"heartbeat"`
	Daemon       DaemonConfig       `json:"daemon"`
}
```

### 2.1 Identity

```go
// Username is the NATS auth identity that scopes all subjects to
// resystems.renotify.{username}.>. Required — the daemon will not
// start without it. May be set via config file, environment
// variable (RENOTIFY_USERNAME), or CLI flag (--username).
//
// Default: none (must be configured).
```

The `daemon_id` is not part of the configuration file. It is
auto-generated on first startup and persisted in
`$XDG_STATE_HOME/renotify/daemon_id`. See the [Naming & Addressing
Analysis](analysis-naming-and-addressing.md) Section 2.3.

### 2.2 Embedded Broker

```go
// BrokerConfig controls the embedded NATS server. When Enabled is
// false, the daemon connects to an external shared broker using
// the SharedBroker settings instead.
type BrokerConfig struct {
	Enabled  bool   `json:"enabled"`   // default: true
	TCPHost  string `json:"tcp_host"`  // default: "127.0.0.1"
	TCPPort  int    `json:"tcp_port"`  // default: 4222
	WSSHost  string `json:"wss_host"`  // default: "0.0.0.0"
	WSSPort  int    `json:"wss_port"`  // default: 4223
	CertFile string `json:"cert_file"` // default: "$XDG_STATE_HOME/renotify/tls/cert.pem"
	KeyFile  string `json:"key_file"`  // default: "$XDG_STATE_HOME/renotify/tls/key.pem"
}
```

| Field | Default | R-CLI | Notes |
| :--- | :--- | :--- | :--- |
| `enabled` | `true` | R-CLI-02 | When false, `shared_broker` settings are used |
| `tcp_host` | `"127.0.0.1"` | R-CLI-01 | Loopback only; CLI and MCP bridge |
| `tcp_port` | `4222` | R-CLI-01 | Standard NATS port |
| `wss_host` | `"0.0.0.0"` | R-CLI-01 | All interfaces; mobile app |
| `wss_port` | `4223` | R-CLI-01 | Transmitted in `ProvisioningPayload.p` |
| `cert_file` | XDG state path | R-CLI-11 | Generated by `renotify pair` |
| `key_file` | XDG state path | R-CLI-11 | Permissions 0600 |

### 2.3 MCP Server

```go
// MCPConfig controls the embedded MCP server. When Enabled is
// false, the daemon does not expose MCP tools to agents.
type MCPConfig struct {
	Enabled bool `json:"enabled"` // default: true
}
```

| Field | Default | R-CLI | Notes |
| :--- | :--- | :--- | :--- |
| `enabled` | `true` | R-CLI-03 | MCP server uses stdio transport (launched as subprocess by agents) |

The MCP server uses the standard MCP stdio transport — agents
launch the daemon (or a dedicated MCP subprocess) and communicate
over stdin/stdout. No separate port or socket configuration is
needed. The MCP server connects to the NATS broker internally
(embedded or shared) using the daemon's own credentials.

### 2.4 JetStream

```go
// JetStreamConfig holds the configurable parameters for the
// RENOTIFY JetStream stream. Non-configurable parameters (storage:
// memory, retention: limits, discard: old, replicas: 1) are
// hardcoded at stream creation.
type JetStreamConfig struct {
	MaxAge         Duration `json:"max_age"`           // default: "30m"
	MaxBytes       int64    `json:"max_bytes"`         // default: 134217728
	MaxMsgSize     int32    `json:"max_msg_size"`      // default: 65536
	MaxMsgsPerSubj int64   `json:"max_msgs_per_subj"` // default: 1000
	DupWindow      Duration `json:"dup_window"`        // default: "2m"
}
```

| Field | Default | R-CLI | Notes |
| :--- | :--- | :--- | :--- |
| `max_age` | `"30m"` | R-CLI-12 | Message TTL; memory-backed |
| `max_bytes` | `134217728` (128 MB) | R-CLI-12 | Total stream memory cap |
| `max_msg_size` | `65536` (64 KB) | R-SYS-01 | Per-message size limit |
| `max_msgs_per_subj` | `1000` | — | Prevents runaway flow consuming all capacity |
| `dup_window` | `"2m"` | — | Dedup window for `Nats-Msg-Id` |

`Duration` is a custom type that marshals to/from a human-readable
string (e.g., `"30m"`, `"2h"`, `"300s"`) in JSON and a
`time.Duration` in Go.

### 2.5 Shared Broker

```go
// SharedBrokerConfig provides connection details for an external
// NATS broker. Used only when broker.enabled is false. All fields
// are ignored when the embedded broker is active.
type SharedBrokerConfig struct {
	URL        string `json:"url"`                    // e.g., "nats://broker.example.com:4222"
	Username   string `json:"username,omitempty"`      // NATS auth username
	Password   string `json:"password,omitempty"`      // NATS auth password
	Token      string `json:"token,omitempty"`         // NATS auth token (alternative to user/pass)
	TLSEnabled bool   `json:"tls_enabled,omitempty"`   // enable TLS for broker connection
	CACert     string `json:"ca_cert,omitempty"`       // CA certificate path (PEM)
	ClientCert string `json:"client_cert,omitempty"`   // client certificate path (PEM, for mTLS)
	ClientKey  string `json:"client_key,omitempty"`    // client private key path (PEM, for mTLS)
}
```

| Field | Default | Notes |
| :--- | :--- | :--- |
| `url` | none | Required when `broker.enabled` is false |
| `username` | none | Basic auth; mutually exclusive with `token` |
| `password` | none | Basic auth |
| `token` | none | Token auth; mutually exclusive with `username`/`password` |
| `tls_enabled` | `false` | Enable TLS for the daemon's connection to the shared broker |
| `ca_cert` | none | For CA-signed broker certificates |
| `client_cert` | none | For mutual TLS |
| `client_key` | none | For mutual TLS |

### 2.6 Operational Parameters

```go
// RateLimitConfig controls per-flow notification rate limiting.
type RateLimitConfig struct {
	NotificationsPerMinute int `json:"notifications_per_minute"` // default: 60
}

// ReapingConfig controls stale flow detection.
type ReapingConfig struct {
	GracePeriod Duration `json:"grace_period"` // default: "5m"
}

// TimeoutConfig controls blocking request timeouts.
type TimeoutConfig struct {
	DefaultAskTimeout Duration `json:"default_ask_timeout"` // default: "5m"
}

// HeartbeatConfig controls daemon heartbeat publishing.
type HeartbeatConfig struct {
	Interval Duration `json:"interval"` // default: "30s"
}
```

| Field | Default | R-CLI | Notes |
| :--- | :--- | :--- | :--- |
| `rate_limit.notifications_per_minute` | `60` | R-CLI-16 | Per-flow; exceeding returns `rate_limited` error |
| `reaping.grace_period` | `"5m"` | R-CLI-18 | Inactivity before flow is marked failed |
| `timeout.default_ask_timeout` | `"5m"` | R-CLI-06 | Overridable per-invocation via `--timeout` flag |
| `heartbeat.interval` | `"30s"` | R-CLI-14 | Periodic `DaemonHeartbeat`; also triggers on state changes |

### 2.7 Daemon Runtime

```go
// DaemonConfig controls daemon process behaviour.
type DaemonConfig struct {
	Foreground bool   `json:"foreground"` // default: false
	LogFile    string `json:"log_file"`   // default: "$XDG_STATE_HOME/renotify/daemon.log"
	DBPath     string `json:"db_path"`    // default: "$XDG_STATE_HOME/renotify/renotify.db"
}
```

| Field | Default | R-CLI | Notes |
| :--- | :--- | :--- | :--- |
| `foreground` | `false` | R-CLI-01 | Overridable via `--foreground` flag |
| `log_file` | XDG state path | R-CLI-01 | Used in background mode; foreground logs to stderr |
| `db_path` | XDG state path | R-CLI-09 | SQLite ledger; see [SQLite Ledger](analysis-sqlite-ledger.md) |

---

## 3. Example `settings.json`

### 3.1 Embedded Broker (Solo Developer)

A solo developer using the embedded broker needs only to set their
username. All other values use compiled defaults.

```json
{
  "username": "stewart"
}
```

With optional tuning:

```json
{
  "username": "stewart",
  "broker": {
    "wss_port": 9443
  },
  "timeout": {
    "default_ask_timeout": "10m"
  },
  "heartbeat": {
    "interval": "15s"
  }
}
```

### 3.2 Shared Broker (Enterprise)

An enterprise developer connecting to a shared NATS broker
disables the embedded broker and provides the shared broker's
connection details.

```json
{
  "username": "stewart",
  "broker": {
    "enabled": false
  },
  "shared_broker": {
    "url": "nats://nats.internal.example.com:4222",
    "token": "org-issued-nats-token",
    "tls_enabled": true,
    "ca_cert": "/etc/renotify/ca.pem"
  }
}
```

---

## 4. Viper Binding Model

The daemon uses [Cobra][cobra] for CLI argument parsing and
[Viper][viper] for layered configuration. The precedence order
(highest to lowest) is:

1. **CLI flag** — explicit per-invocation override
   (e.g., `--timeout 10m`)
2. **Environment variable** — process-level override
   (e.g., `RENOTIFY_TIMEOUT_DEFAULT_ASK_TIMEOUT=10m`)
3. **Config file** — `settings.json` values
4. **Compiled default** — hardcoded in the Go source

### 4.1 Environment Variable Convention

All environment variables use the `RENOTIFY_` prefix with
underscore-separated nesting matching the JSON key path. Viper's
`AutomaticEnv()` with `SetEnvPrefix("RENOTIFY")` and
`SetEnvKeyReplacer(strings.NewReplacer(".", "_"))` handles this
automatically.

| Config Key | Environment Variable |
| :--- | :--- |
| `username` | `RENOTIFY_USERNAME` |
| `broker.enabled` | `RENOTIFY_BROKER_ENABLED` |
| `broker.tcp_port` | `RENOTIFY_BROKER_TCP_PORT` |
| `broker.wss_port` | `RENOTIFY_BROKER_WSS_PORT` |
| `jetstream.max_age` | `RENOTIFY_JETSTREAM_MAX_AGE` |
| `shared_broker.url` | `RENOTIFY_SHARED_BROKER_URL` |
| `rate_limit.notifications_per_minute` | `RENOTIFY_RATE_LIMIT_NOTIFICATIONS_PER_MINUTE` |
| `reaping.grace_period` | `RENOTIFY_REAPING_GRACE_PERIOD` |
| `timeout.default_ask_timeout` | `RENOTIFY_TIMEOUT_DEFAULT_ASK_TIMEOUT` |
| `heartbeat.interval` | `RENOTIFY_HEARTBEAT_INTERVAL` |
| `daemon.foreground` | `RENOTIFY_DAEMON_FOREGROUND` |
| `daemon.db_path` | `RENOTIFY_DAEMON_DB_PATH` |

### 4.2 CLI Flag Mapping

Only a subset of configuration values are overridable via CLI
flags. This keeps the flag surface small and avoids exposing
daemon-internal parameters to command-line users.

| Command | Flag | Config Key | Type |
| :--- | :--- | :--- | :--- |
| `renotify daemon` | `--foreground` | `daemon.foreground` | bool |
| `renotify daemon` | `--username` | `username` | string |
| `renotify daemon` | `--config` | (Viper config path) | filepath |
| `renotify ask` | `--timeout` | `timeout.default_ask_timeout` | duration |
| `renotify ask` | `--title` | (per-invocation, not persisted) | string |
| `renotify ask` | `--body` | (per-invocation, not persisted) | string |
| `renotify ask` | `--priority` | (per-invocation, not persisted) | enum |
| `renotify ask` | `--actions` | (per-invocation, not persisted) | []string |
| `renotify ask` | `--response-types` | (per-invocation, not persisted) | []enum |
| `renotify post` | `--title` | (per-invocation, not persisted) | string |
| `renotify post` | `--body` | (per-invocation, not persisted) | string |
| `renotify post` | `--priority` | (per-invocation, not persisted) | enum |
| `renotify post` | `--source` | (per-invocation, not persisted) | string |
| `renotify pair` | `--ip` | (per-invocation, not persisted) | string |
| `renotify pair` | `--regenerate-cert` | (per-invocation, not persisted) | bool |
| `renotify history` | `--workspace-id` | (per-invocation, not persisted) | string |
| `renotify history` | `--flow-id` | (per-invocation, not persisted) | string |
| `renotify history` | `--since` | (per-invocation, not persisted) | RFC 3339 |
| `renotify history` | `--until` | (per-invocation, not persisted) | RFC 3339 |
| `renotify history` | `--limit` | (per-invocation, not persisted) | int |

Flags marked "per-invocation, not persisted" are command arguments
that do not map to `settings.json` keys. They are parsed by Cobra
and passed directly to the command handler.

---

## 5. Validation Rules

The daemon validates the merged configuration (after applying all
precedence layers) at startup. Validation failures cause the daemon
to exit with a non-zero exit code and a descriptive error message
to stderr.

| Parameter | Type | Constraint | Error |
| :--- | :--- | :--- | :--- |
| `username` | string | Non-empty, alphanumeric (matching NATS auth identity) | "username is required" |
| `broker.enabled` | bool | — | — |
| `broker.tcp_port` | int | 1–65535 | "tcp_port out of range" |
| `broker.wss_port` | int | 1–65535; must differ from `tcp_port` | "wss_port out of range" / "wss_port must differ from tcp_port" |
| `broker.cert_file` | filepath | File must exist (checked at WSS listener startup, not config load) | Warning logged; WSS listener skipped |
| `broker.key_file` | filepath | File must exist (checked at WSS listener startup) | Warning logged; WSS listener skipped |
| `shared_broker.url` | string | Required when `broker.enabled` is false; must be a valid NATS URL | "shared_broker.url is required when embedded broker is disabled" |
| `shared_broker.username` + `token` | string | Mutually exclusive (cannot set both) | "shared_broker: username/password and token are mutually exclusive" |
| `jetstream.max_age` | duration | > 0 | "jetstream.max_age must be positive" |
| `jetstream.max_bytes` | int64 | > 0 | "jetstream.max_bytes must be positive" |
| `jetstream.max_msg_size` | int32 | 1–1048576 (1 MB ceiling) | "jetstream.max_msg_size out of range" |
| `jetstream.max_msgs_per_subj` | int64 | > 0 | "jetstream.max_msgs_per_subj must be positive" |
| `jetstream.dup_window` | duration | > 0; ≤ `max_age` | "dup_window must not exceed max_age" |
| `rate_limit.notifications_per_minute` | int | > 0 | "notifications_per_minute must be positive" |
| `reaping.grace_period` | duration | ≥ 1m | "grace_period must be at least 1 minute" |
| `timeout.default_ask_timeout` | duration | > 0 | "default_ask_timeout must be positive" |
| `heartbeat.interval` | duration | ≥ 5s | "heartbeat.interval must be at least 5 seconds" |
| `daemon.log_file` | filepath | Parent directory must exist (checked at startup in background mode) | "log directory does not exist" |
| `daemon.db_path` | filepath | Parent directory must exist (checked at startup) | "database directory does not exist" |

### 5.1 Duration Encoding

Duration values in `settings.json` use human-readable strings
parsed by Go's `time.ParseDuration` with the following units:

* `s` — seconds (e.g., `"30s"`)
* `m` — minutes (e.g., `"5m"`)
* `h` — hours (e.g., `"2h"`)

Combinations are supported (e.g., `"1h30m"`). Bare integers are
not accepted — the unit suffix is required.

In environment variables, the same string format applies (e.g.,
`RENOTIFY_REAPING_GRACE_PERIOD=5m`).

---

## 6. Compiled Defaults Summary

The following table lists every configurable parameter with its
compiled default. These values apply when no config file,
environment variable, or CLI flag provides an override.

| Key | Default | Unit |
| :--- | :--- | :--- |
| `username` | (none — required) | string |
| `broker.enabled` | `true` | bool |
| `broker.tcp_host` | `"127.0.0.1"` | IP |
| `broker.tcp_port` | `4222` | port |
| `broker.wss_host` | `"0.0.0.0"` | IP |
| `broker.wss_port` | `4223` | port |
| `broker.cert_file` | `"$XDG_STATE_HOME/renotify/tls/cert.pem"` | filepath |
| `broker.key_file` | `"$XDG_STATE_HOME/renotify/tls/key.pem"` | filepath |
| `mcp.enabled` | `true` | bool |
| `jetstream.max_age` | `"30m"` | duration |
| `jetstream.max_bytes` | `134217728` | bytes (128 MB) |
| `jetstream.max_msg_size` | `65536` | bytes (64 KB) |
| `jetstream.max_msgs_per_subj` | `1000` | count |
| `jetstream.dup_window` | `"2m"` | duration |
| `shared_broker.url` | (none) | NATS URL |
| `shared_broker.tls_enabled` | `false` | bool |
| `rate_limit.notifications_per_minute` | `60` | count |
| `reaping.grace_period` | `"5m"` | duration |
| `timeout.default_ask_timeout` | `"5m"` | duration |
| `heartbeat.interval` | `"30s"` | duration |
| `daemon.foreground` | `false` | bool |
| `daemon.log_file` | `"$XDG_STATE_HOME/renotify/daemon.log"` | filepath |
| `daemon.db_path` | `"$XDG_STATE_HOME/renotify/renotify.db"` | filepath |

[xdg-basedir]: https://specifications.freedesktop.org/basedir-spec/latest/
[cobra]: https://github.com/spf13/cobra
[viper]: https://github.com/spf13/viper
