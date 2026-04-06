// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/cli/internal/config"
	"go.resystems.io/renotify/cli/internal/exitcode"
	"go.resystems.io/renotify/cli/internal/state"
	"go.resystems.io/renotify/cli/internal/xdg"
)

func newRevokeCmd(app *App) *cobra.Command {
	var (
		format   string
		deviceID string
		all      bool
	)

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke a mobile device pairing",
		Long: `Remove a paired mobile device and disconnect it from the daemon.

With --device <device_id>, revokes a specific device. With --all,
revokes all paired devices. With no flags and a single device
paired, revokes that device. With multiple devices and no flags,
returns an error asking you to specify.

Use 'renotify pairings' to list paired devices and their IDs.

This command is idempotent: revoking a non-existent device prints
a message and exits successfully.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config
			devicesPath := xdg.DevicesPath()

			// Migrate legacy single-token if needed.
			state.MigrateFromSingleToken(
				xdg.PairingTokenPath(),
				xdg.PairingUsernamePath(),
				devicesPath)

			devices, err := state.LoadDevices(devicesPath)
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"load devices: %v", err)
			}

			if all {
				return revokeAll(cmd, cfg, devicesPath,
					devices, format)
			}

			if deviceID != "" {
				return revokeDevice(cmd, cfg, devicesPath,
					deviceID, format)
			}

			// No flags: revoke the single device, or error
			// if ambiguous.
			switch len(devices) {
			case 0:
				return writeRevokeOutput(cmd, format, false,
					"no paired devices", cfg.Broker.Enabled)
			case 1:
				return revokeDevice(cmd, cfg, devicesPath,
					devices[0].DeviceID, format)
			default:
				return exitcode.Errorf(exitcode.Usage,
					"multiple devices paired; use "+
						"--device <id> or --all "+
						"(see 'renotify pairings')")
			}
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	cmd.Flags().StringVarP(&deviceID, "device", "d", "",
		"revoke a specific device by ID")
	cmd.Flags().BoolVar(&all, "all", false,
		"revoke all paired devices")

	return cmd
}

func revokeDevice(
	cmd *cobra.Command,
	cfg *config.Config,
	devicesPath, deviceID, format string,
) error {
	removed, err := state.RemoveDevice(devicesPath, deviceID)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "%v", err)
	}
	if !removed {
		return writeRevokeOutput(cmd, format, false,
			fmt.Sprintf("device %s not found", deviceID),
			cfg.Broker.Enabled)
	}

	signalAndLog(cmd, cfg)

	return writeRevokeOutput(cmd, format, true,
		fmt.Sprintf("device %s revoked", deviceID),
		cfg.Broker.Enabled)
}

func revokeAll(
	cmd *cobra.Command,
	cfg *config.Config,
	devicesPath string,
	devices []state.PairedDevice,
	format string,
) error {
	if len(devices) == 0 {
		return writeRevokeOutput(cmd, format, false,
			"no paired devices", cfg.Broker.Enabled)
	}

	if err := state.ClearDevices(devicesPath); err != nil {
		return exitcode.Errorf(exitcode.Error, "%v", err)
	}

	signalAndLog(cmd, cfg)

	msg := fmt.Sprintf("%d device(s) revoked", len(devices))
	return writeRevokeOutput(cmd, format, true,
		msg, cfg.Broker.Enabled)
}

func signalAndLog(cmd *cobra.Command, cfg *config.Config) {
	if !cfg.Broker.Enabled {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"Shared broker mode: local token deleted. "+
				"The operator must revoke on the broker side.")
		return
	}

	result, pid := signalDaemonReload()
	switch result {
	case signalSent:
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Daemon (PID %d) notified — device "+
				"will be disconnected.\n", pid)
	case signalNoDaemon:
		fmt.Fprintln(cmd.ErrOrStderr(),
			"Note: no running daemon detected. "+
				"Device revoked from disk.")
	case signalFailed:
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Warning: could not signal daemon "+
				"(PID %d). Device revoked from disk but "+
				"daemon still has old auth.\n", pid)
	}
}

type revokeJSONOutput struct {
	Revoked  bool   `json:"revoked"`
	Message  string `json:"message"`
	Embedded bool   `json:"embedded_broker"`
}

func writeRevokeOutput(cmd *cobra.Command, format string,
	revoked bool, message string, embedded bool) error {

	switch format {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(revokeJSONOutput{
			Revoked:  revoked,
			Message:  message,
			Embedded: embedded,
		})
	default:
		if revoked {
			fmt.Fprintln(cmd.ErrOrStderr(), message)
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), message)
		}
		return nil
	}
}
