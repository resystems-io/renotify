package command

import (
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/ledger"
	"go.resystems.io/renotify/internal/registry"
)

func newFlowCmd(app *App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "flow <flow_id>",
		Short: "Show details of a single active flow",
		Long: `Query the daemon for a specific active flow and display its
full details including metadata. The flow ID is obtained from
the 'renotify flows' command or from register_flow MCP tool
output.

Examples:

  renotify flow fl_06EMJN96M1WCQFWQXQ9GBT2ZPR
  renotify flow --format json fl_06EMJN96M1WCQFWQXQ9GBT2ZPR`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flowID := args[0]
			cfg := app.Config

			nc, err := broker.ConnectCLI(cfg)
			if err != nil {
				return exitcode.Errorf(exitcode.Error, "%v", err)
			}
			defer nc.Drain()

			query := ledger.ActiveFlowsQuery{
				FlowID: flowID,
			}
			queryData, _ := json.Marshal(query)

			subject := broker.ServiceFlowsSubject(cfg.Username)
			resp, err := nc.Request(subject, queryData,
				2*time.Second)
			if err != nil {
				if err == nats.ErrTimeout {
					return exitcode.Errorf(exitcode.Error,
						"daemon not responding (is it running?)")
				}
				return exitcode.Errorf(exitcode.Error,
					"query flow: %v", err)
			}

			var result registry.ActiveFlowsResult
			if err := json.Unmarshal(resp.Data, &result); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"parse response: %v", err)
			}

			if len(result.Flows) == 0 {
				return exitcode.Errorf(exitcode.NotFound,
					"flow %s not found (not active)", flowID)
			}

			f := result.Flows[0]

			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				return enc.Encode(f)
			}

			gracePeriod := cfg.Reaping.GracePeriod.Duration
			now := time.Now()
			ttl := f.LastActivityTimestamp.Add(
				gracePeriod).Sub(now)

			w := tabwriter.NewWriter(cmd.OutOrStdout(),
				0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "Flow ID:\t%s\n", f.FlowID)
			fmt.Fprintf(w, "Daemon ID:\t%s\n", f.DaemonID)
			fmt.Fprintf(w, "Workspace ID:\t%s\n", f.WorkspaceID)

			ws := f.DisplayName
			if ws == "" {
				ws = "-"
			}
			fmt.Fprintf(w, "Workspace:\t%s\n", ws)

			if f.AbsPath != "" {
				fmt.Fprintf(w, "Path:\t%s\n", f.AbsPath)
			}

			label := f.Label
			if label == "" {
				label = "-"
			}
			fmt.Fprintf(w, "Label:\t%s\n", label)
			fmt.Fprintf(w, "Registered:\t%s\n",
				f.RegisteredAt.Format(time.RFC3339))
			fmt.Fprintf(w, "Last Activity:\t%s\n",
				f.LastActivityTimestamp.Format(time.RFC3339))
			fmt.Fprintf(w, "TTL:\t%s\n", formatTTL(ttl))

			if len(f.Metadata) > 0 {
				keys := make([]string, 0, len(f.Metadata))
				for k := range f.Metadata {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				fmt.Fprintf(w, "\nMetadata:\n")
				for _, k := range keys {
					fmt.Fprintf(w, "  %s:\t%s\n", k, f.Metadata[k])
				}
			}

			w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
