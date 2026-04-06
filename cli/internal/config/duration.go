// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package config defines the unified daemon configuration schema.
// See docs/analysis-configuration-schema.md.
package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration wraps time.Duration with human-readable JSON
// marshaling. Values are encoded as strings (e.g., "30m", "5s",
// "1h30m") matching time.ParseDuration syntax. Bare integers are
// rejected — the unit suffix is required.
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("duration must be a string (e.g. %q): %w", "30m", err)
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = dur
	return nil
}

// MarshalText implements encoding.TextMarshaler for Viper
// compatibility with environment variable binding.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler for Viper
// compatibility with environment variable binding.
func (d *Duration) UnmarshalText(b []byte) error {
	dur, err := time.ParseDuration(string(b))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(b), err)
	}
	d.Duration = dur
	return nil
}

// NewDuration creates a Duration from a time.Duration.
func NewDuration(d time.Duration) Duration {
	return Duration{Duration: d}
}
