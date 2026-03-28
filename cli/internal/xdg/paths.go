// Package xdg resolves XDG Base Directory paths for Renotify.
// See docs/analysis-configuration-schema.md Section 1.
package xdg

import (
	"os"
	"path/filepath"
)

const appName = "renotify"

// ConfigHome returns $XDG_CONFIG_HOME/renotify, defaulting to
// ~/.config/renotify if XDG_CONFIG_HOME is unset.
func ConfigHome() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, appName)
}

// StateHome returns $XDG_STATE_HOME/renotify, defaulting to
// ~/.local/state/renotify if XDG_STATE_HOME is unset.
func StateHome() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, appName)
}

// ConfigFilePath returns the path to settings.json.
func ConfigFilePath() string {
	return filepath.Join(ConfigHome(), "settings.json")
}

// DaemonIDPath returns the path to the persisted daemon_id file.
func DaemonIDPath() string {
	return filepath.Join(StateHome(), "daemon_id")
}

// InternalTokenPath returns the path to the embedded broker's
// internal auth token.
func InternalTokenPath() string {
	return filepath.Join(StateHome(), "internal_token")
}

// DBPath returns the default path to the SQLite ledger database.
func DBPath() string {
	return filepath.Join(StateHome(), "renotify.db")
}

// DaemonLogPath returns the default path to the daemon log file
// (used in background mode).
func DaemonLogPath() string {
	return filepath.Join(StateHome(), "daemon.log")
}

// TLSDir returns the directory containing TLS cert and key.
func TLSDir() string {
	return filepath.Join(StateHome(), "tls")
}

// TLSCertPath returns the path to the self-signed TLS certificate.
func TLSCertPath() string {
	return filepath.Join(TLSDir(), "cert.pem")
}

// TLSKeyPath returns the path to the TLS private key.
func TLSKeyPath() string {
	return filepath.Join(TLSDir(), "key.pem")
}

// PairingDir returns the directory containing pairing artifacts.
func PairingDir() string {
	return filepath.Join(StateHome(), "pairing")
}

// PairingTokenPath returns the path to the mobile auth token.
func PairingTokenPath() string {
	return filepath.Join(PairingDir(), "token")
}

// PairingUsernamePath returns the path to the pairing username.
func PairingUsernamePath() string {
	return filepath.Join(PairingDir(), "username")
}

// EnsureDir creates a directory and all parents with mode 0700 if
// it does not already exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0700)
}
