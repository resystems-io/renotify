package command

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/state"
	"go.resystems.io/renotify/internal/xdg"
)

// deviceControl is the control message published to a device's
// control subject via Core NATS Pub/Sub.
type deviceControl struct {
	Command   string `json:"command"`
	Value     bool   `json:"value"`
	Timestamp string `json:"timestamp"`
}

func newSilentCmd(app *App) *cobra.Command {
	var (
		deviceID string
		all      bool
		format   string
	)

	cmd := &cobra.Command{
		Use:   "silent <on|off>",
		Short: "Remotely toggle silent mode on a paired device",
		Long: `Send a control command to a paired mobile device to enable
or disable notification silent mode. When silent, the device
still receives and acknowledges notifications but does not
render them.

Specify a device with --device or target all paired devices
with --all. Use 'renotify pairings' to list device IDs.`,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config

			// Parse on/off argument.
			var silent bool
			switch args[0] {
			case "on":
				silent = true
			case "off":
				silent = false
			default:
				return exitcode.Errorf(exitcode.Usage,
					"argument must be 'on' or 'off', got %q",
					args[0])
			}

			// Load paired devices.
			devices, err := state.LoadDevices(
				xdg.DevicesPath())
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"load devices: %v", err)
			}
			if len(devices) == 0 {
				return exitcode.Errorf(exitcode.Error,
					"no paired devices")
			}

			// Determine target devices.
			var targets []state.PairedDevice
			switch {
			case all:
				targets = devices
			case deviceID != "":
				for _, d := range devices {
					if d.DeviceID == deviceID {
						targets = append(targets, d)
						break
					}
				}
				if len(targets) == 0 {
					return exitcode.Errorf(exitcode.Error,
						"device %q not found", deviceID)
				}
			default:
				if len(devices) == 1 {
					targets = devices
				} else {
					fmt.Fprintln(cmd.ErrOrStderr(),
						"Multiple devices paired. "+
							"Specify --device or --all:")
					for _, d := range devices {
						fmt.Fprintf(cmd.ErrOrStderr(),
							"  %s\n", d.DeviceID)
					}
					return exitcode.Errorf(exitcode.Usage,
						"--device or --all required")
				}
			}

			// Connect to daemon NATS.
			nc, err := broker.ConnectCLI(cfg)
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"%v", err)
			}
			defer nc.Drain()

			// Publish control message to each target.
			msg := deviceControl{
				Command: "set_silent",
				Value:   silent,
				Timestamp: time.Now().UTC().Format(
					time.RFC3339),
			}
			data, _ := json.Marshal(msg)

			type silentResult struct {
				DeviceID string `json:"device_id"`
				Silent   bool   `json:"silent"`
			}
			var results []silentResult

			for _, d := range targets {
				subject := broker.DeviceControlSubject(
					cfg.Username, d.DeviceID)
				if err := nc.Publish(subject, data); err != nil {
					return exitcode.Errorf(exitcode.Error,
						"publish to %s: %v",
						d.DeviceID, err)
				}
				results = append(results, silentResult{
					DeviceID: d.DeviceID,
					Silent:   silent,
				})
			}

			nc.Flush()

			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			for _, r := range results {
				stateStr := "on"
				if !r.Silent {
					stateStr = "off"
				}
				fmt.Fprintf(cmd.OutOrStdout(),
					"Silent mode set to %s for %s\n",
					stateStr, r.DeviceID)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&deviceID, "device", "d", "",
		"target device ID")
	cmd.Flags().BoolVar(&all, "all", false,
		"target all paired devices")
	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
