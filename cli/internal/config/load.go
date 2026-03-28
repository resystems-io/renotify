package config

import (
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"go.resystems.io/renotify/internal/xdg"
)

// Load reads configuration from the settings.json file (if it
// exists), environment variables, and compiled defaults. The
// configPath argument overrides the default XDG config file path
// (used by the --config CLI flag). Pass "" to use the default.
//
// Precedence (highest to lowest):
//  1. CLI flags (bound by the caller after Load returns)
//  2. Environment variables (RENOTIFY_ prefix)
//  3. Config file (settings.json)
//  4. Compiled defaults
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Start from compiled defaults.
	cfg := Default()
	setDefaults(v, cfg)

	// Config file. The file is optional — the daemon starts with
	// compiled defaults if settings.json does not exist.
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("settings")
		v.SetConfigType("json")
		v.AddConfigPath(xdg.ConfigHome())
	}
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// File exists but is malformed or unreadable.
			if !os.IsNotExist(err) {
				return nil, err
			}
		}
	}

	// Environment variables: RENOTIFY_ prefix, dots become
	// underscores (e.g., broker.tcp_port → RENOTIFY_BROKER_TCP_PORT).
	v.SetEnvPrefix("RENOTIFY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Unmarshal into the config struct. The custom decode hook
	// teaches mapstructure how to convert string values (from
	// Viper defaults and env vars) into our Duration type.
	if err := v.Unmarshal(cfg, viper.DecoderConfigOption(
		func(dc *mapstructure.DecoderConfig) {
			dc.DecodeHook = mapstructure.ComposeDecodeHookFunc(
				dc.DecodeHook,
				stringToDurationHook(),
			)
		},
	)); err != nil {
		return nil, err
	}

	return cfg, nil
}

// stringToDurationHook returns a mapstructure decode hook that
// converts string values to our custom Duration type using
// time.ParseDuration.
func stringToDurationHook() mapstructure.DecodeHookFuncType {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{},
	) (interface{}, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(Duration{}) {
			return data, nil
		}
		d, err := time.ParseDuration(data.(string))
		if err != nil {
			return nil, err
		}
		return Duration{Duration: d}, nil
	}
}

// setDefaults registers compiled defaults with Viper so they
// participate in the merge.
func setDefaults(v *viper.Viper, cfg *Config) {
	v.SetDefault("username", cfg.Username)

	v.SetDefault("broker.enabled", cfg.Broker.Enabled)
	v.SetDefault("broker.tcp_host", cfg.Broker.TCPHost)
	v.SetDefault("broker.tcp_port", cfg.Broker.TCPPort)
	v.SetDefault("broker.wss_host", cfg.Broker.WSSHost)
	v.SetDefault("broker.wss_port", cfg.Broker.WSSPort)
	v.SetDefault("broker.cert_file", cfg.Broker.CertFile)
	v.SetDefault("broker.key_file", cfg.Broker.KeyFile)

	v.SetDefault("mcp.enabled", cfg.MCP.Enabled)

	v.SetDefault("jetstream.max_age", cfg.JetStream.MaxAge.Duration.String())
	v.SetDefault("jetstream.max_bytes", cfg.JetStream.MaxBytes)
	v.SetDefault("jetstream.max_msg_size", cfg.JetStream.MaxMsgSize)
	v.SetDefault("jetstream.max_msgs_per_subj", cfg.JetStream.MaxMsgsPerSubj)
	v.SetDefault("jetstream.dup_window", cfg.JetStream.DupWindow.Duration.String())

	v.SetDefault("shared_broker.url", cfg.SharedBroker.URL)
	v.SetDefault("shared_broker.tls_enabled", cfg.SharedBroker.TLSEnabled)

	v.SetDefault("rate_limit.notifications_per_minute", cfg.RateLimit.NotificationsPerMinute)
	v.SetDefault("reaping.grace_period", cfg.Reaping.GracePeriod.Duration.String())
	v.SetDefault("timeout.default_ask_timeout", cfg.Timeout.DefaultAskTimeout.Duration.String())
	v.SetDefault("heartbeat.interval", cfg.Heartbeat.Interval.Duration.String())
	v.SetDefault("interjection.debounce_window", cfg.Interjection.DebounceWindow.Duration.String())

	v.SetDefault("daemon.foreground", cfg.Daemon.Foreground)
	v.SetDefault("daemon.log_file", cfg.Daemon.LogFile)
	v.SetDefault("daemon.db_path", cfg.Daemon.DBPath)
}
