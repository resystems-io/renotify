// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package state

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// PairedDevice represents a single mobile device paired with
// the daemon. Stored in devices.json.
type PairedDevice struct {
	DeviceID string    `json:"device_id"`
	Token    string    `json:"token"`
	PairedAt time.Time `json:"paired_at"`
}

// LoadDevices reads the device registry from path. Returns an
// empty slice if the file does not exist.
func LoadDevices(path string) ([]PairedDevice, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read devices: %w", err)
	}

	var devices []PairedDevice
	if err := json.Unmarshal(data, &devices); err != nil {
		return nil, fmt.Errorf("parse devices: %w", err)
	}
	return devices, nil
}

// SaveDevices writes the device registry atomically.
func SaveDevices(path string, devices []PairedDevice) error {
	data, err := json.MarshalIndent(devices, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal devices: %w", err)
	}
	data = append(data, '\n')
	return writeAtomicBytes(path, data, 0600)
}

// AddDevice appends a device to the registry.
func AddDevice(path string, d PairedDevice) error {
	devices, err := LoadDevices(path)
	if err != nil {
		return err
	}
	devices = append(devices, d)
	return SaveDevices(path, devices)
}

// RemoveDevice removes a device by ID from the registry.
// Returns true if a device was removed.
func RemoveDevice(path string, deviceID string) (bool, error) {
	devices, err := LoadDevices(path)
	if err != nil {
		return false, err
	}

	filtered := devices[:0]
	found := false
	for _, d := range devices {
		if d.DeviceID == deviceID {
			found = true
			continue
		}
		filtered = append(filtered, d)
	}
	if !found {
		return false, nil
	}

	return true, SaveDevices(path, filtered)
}

// ClearDevices removes all devices from the registry.
func ClearDevices(path string) error {
	return SaveDevices(path, []PairedDevice{})
}

// MigrateFromSingleToken converts the legacy single-token
// pairing to a devices.json entry. If devices.json already
// exists, this is a no-op. If the legacy token file does not
// exist, this is a no-op.
func MigrateFromSingleToken(
	tokenPath, usernamePath, devicesPath string,
) error {
	// Skip if devices.json already exists.
	if _, err := os.Stat(devicesPath); err == nil {
		return nil
	}

	// Read legacy token.
	token, err := LoadPairingToken(tokenPath)
	if err != nil || token == "" {
		return nil // no legacy token — nothing to migrate
	}

	// Generate a device_id for the legacy device.
	deviceID, err := GenerateDeviceID()
	if err != nil {
		return err
	}

	devices := []PairedDevice{{
		DeviceID: deviceID,
		Token:    token,
		PairedAt: time.Now().UTC(),
	}}

	if err := SaveDevices(devicesPath, devices); err != nil {
		return err
	}

	// Remove legacy files.
	os.Remove(tokenPath)
	os.Remove(usernamePath)

	return nil
}

// NatsUsername returns the NATS auth username for a device.
func NatsUsername(deviceID string) string {
	return "mobile-" + deviceID
}

// LoadPairingUsername reads the pairing username from path.
// Returns ("", nil) if the file does not exist.
func LoadPairingUsername(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read pairing username: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
