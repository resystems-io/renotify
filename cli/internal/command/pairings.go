package command

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/state"
	"go.resystems.io/renotify/internal/xdg"
)

func newPairingsCmd(app *App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "pairings",
		Short: "List all paired mobile devices",
		Long: `Display all mobile devices currently paired with this daemon.
Each device has a unique device_id and its own auth token.

Use 'renotify revoke --device <device_id>' to revoke a
specific device, or 'renotify revoke --all' to revoke all.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			devices, err := state.LoadDevices(
				xdg.DevicesPath())
			if err != nil {
				return fmt.Errorf("load devices: %w", err)
			}

			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if devices == nil {
					devices = []state.PairedDevice{}
				}
				return enc.Encode(devices)
			}

			if len(devices) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"No paired devices.")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(),
				0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "DEVICE ID\tPAIRED AT\n")
			for _, d := range devices {
				fmt.Fprintf(w, "%s\t%s\n",
					d.DeviceID,
					d.PairedAt.Format(time.RFC3339))
			}
			w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: text or json")

	return cmd
}
