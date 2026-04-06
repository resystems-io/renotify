// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package config

import (
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"go.resystems.io/renotify/cli/internal/xdg"
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
// participate in the merge and env var binding. The Registry is
// the single source of truth for key paths; Default() is the
// single source of truth for values.
func setDefaults(v *viper.Viper, cfg *Config) {
	for _, p := range Registry {
		val := p.Resolve(cfg)
		// Viper needs Duration defaults as strings for env var
		// binding and the mapstructure decode hook.
		if d, ok := val.(Duration); ok {
			val = d.Duration.String()
		}
		v.SetDefault(p.Key, val)
	}
}
