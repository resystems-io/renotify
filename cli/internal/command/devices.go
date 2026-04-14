// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/exitcode"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

func newDevicesCmd(app *App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "devices",
		Short: "Show paired devices with connectivity status",
		Long: `Query the running daemon for the connectivity status of all
paired mobile devices. Each device is shown with its online/offline
state and last-seen timestamp.

This command requires a running daemon. For a file-only view of
paired devices (no daemon required), use 'renotify pairings'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDevices(app, cmd, format)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}

func runDevices(app *App, cmd *cobra.Command, format string) error {
	cfg := app.Config

	nc, err := broker.ConnectCLI(cfg)
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"cannot connect to daemon: %v\n"+
				"Is the daemon running? Check with: renotify daemon status",
			err)
	}
	defer nc.Close()

	subject := broker.ServiceDevicePresenceSubject(cfg.Username)
	msg, err := nc.Request(subject, nil, 5*time.Second)
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"device presence query failed: %v", err)
	}

	var result statesvc.DevicePresenceResult
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		return exitcode.Errorf(exitcode.Error,
			"invalid response: %v", err)
	}

	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(result)
	}

	if len(result.Devices) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"No paired devices.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(),
		0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "USERNAME\tDEVICE ID\tSTATUS\tLAST SEEN\tPAIRED AT\n")
	for _, d := range result.Devices {
		status := "offline"
		if d.Online {
			status = "online"
		}
		lastSeen := "-"
		if d.LastSeen != nil {
			lastSeen = d.LastSeen.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			d.Username,
			d.DeviceID,
			status,
			lastSeen,
			d.PairedAt.Format(time.RFC3339))
	}
	w.Flush()
	return nil
}
