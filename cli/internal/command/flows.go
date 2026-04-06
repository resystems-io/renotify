// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/exitcode"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

func newFlowsCmd(app *App) *cobra.Command {
	var (
		format    string
		workspace string
	)

	cmd := &cobra.Command{
		Use:   "flows",
		Short: "List active flows",
		Long: `Query the daemon for currently active flows. Shows flow IDs,
workspace names, labels, registration time, and time remaining
before stale reaping (TTL).

Use flow IDs with other commands:
  renotify interject -f <flow_id> note -m "message"
  renotify answer -f <flow_id> -n <notification_id> --accepted`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config

			nc, err := broker.ConnectCLI(cfg)
			if err != nil {
				return exitcode.Errorf(exitcode.Error, "%v", err)
			}
			defer nc.Drain()

			// Build query.
			query := statesvc.FlowsQuery{
				WorkspaceID: workspace,
			}
			queryData, _ := json.Marshal(query)

			// Request active flows from daemon.
			subject := broker.ServiceFlowsSubject(cfg.Username)
			resp, err := nc.Request(subject, queryData,
				2*time.Second)
			if err != nil {
				if err == nats.ErrTimeout {
					return exitcode.Errorf(exitcode.Error,
						"daemon not responding (is it running?)")
				}
				return exitcode.Errorf(exitcode.Error,
					"query flows: %v", err)
			}

			var result statesvc.FlowsResult
			if err := json.Unmarshal(resp.Data, &result); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"parse response: %v", err)
			}

			// Output.
			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			if len(result.Flows) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No active flows.")
				return nil
			}

			gracePeriod := cfg.Reaping.GracePeriod.Duration
			now := time.Now()

			w := tabwriter.NewWriter(cmd.OutOrStdout(),
				0, 0, 2, ' ', 0)
			fmt.Fprintln(w,
				"FLOW ID\tWORKSPACE\tLABEL\tREGISTERED\tTTL")
			for _, f := range result.Flows {
				ttl := f.LastActivityTimestamp.Add(
					gracePeriod).Sub(now)
				ttlStr := formatTTL(ttl)
				label := f.Label
				if label == "" {
					label = "-"
				}
				ws := f.DisplayName
				if ws == "" {
					ws = f.WorkspaceID
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					f.FlowID,
					ws,
					label,
					f.RegisteredAt.Format(time.RFC3339),
					ttlStr,
				)
			}
			w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")
	cmd.Flags().StringVarP(&workspace, "workspace", "w", "",
		"filter by workspace ID")

	return cmd
}

// formatTTL formats a duration as a human-readable TTL string.
// Negative durations show "expired".
func formatTTL(d time.Duration) string {
	if d <= 0 {
		return "expired"
	}
	if d >= time.Hour {
		return fmt.Sprintf("%dh%dm",
			int(d.Hours()), int(d.Minutes())%60)
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm%ds",
			int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
