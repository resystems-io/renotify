// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/exitcode"
	"go.resystems.io/renotify/cli/internal/statesvc"
)

func newFlowCmd(app *App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "flow <flow_id_or_workspace>",
		Short: "Show details of active flow(s)",
		Long: `Query the daemon for active flows and display their
full details including metadata. You can search by:
  - Flow ID (starts with fl_)
  - Workspace ID (starts with ws_)
  - Workspace Path (absolute or containing slashes)
  - Workspace Name (basename)

Examples:

  renotify flow fl_06EMJN96M1WCQFWQXQ9GBT2ZPR
  renotify flow ws_06EMJN96M1WCQFWQXQ9GBT2ZPR
  renotify flow /home/user/project
  renotify flow myproject
  renotify flow --format json myproject`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := args[0]
			cfg := app.Config

			nc, err := broker.ConnectCLI(cfg)
			if err != nil {
				return exitcode.Errorf(exitcode.Error, "%v", err)
			}
			defer nc.Drain()

			var flowIDs []string

			if strings.HasPrefix(arg, "fl_") || strings.HasPrefix(arg, "ws_") {
				// Strict programmatic lookup
				flowIDs = []string{arg} // we handle hydration below by just passing it as the argument
			} else {
				// Broad search lookup
				searchQuery := statesvc.SearchFlowsQuery{Workspace: arg}
				searchData, _ := json.Marshal(searchQuery)
				searchSubj := broker.ServiceSearchFlowsSubject(cfg.Username)
				resp, err := nc.Request(searchSubj, searchData, 2*time.Second)
				if err != nil {
					if err == nats.ErrTimeout {
						return exitcode.Errorf(exitcode.Error, "daemon not responding (is it running?)")
					}
					return exitcode.Errorf(exitcode.Error, "search flows: %v", err)
				}
				var searchRes statesvc.SearchFlowsResult
				if err := json.Unmarshal(resp.Data, &searchRes); err != nil {
					return exitcode.Errorf(exitcode.Error, "parse search response: %v", err)
				}
				flowIDs = searchRes.FlowIDs
			}

			if len(flowIDs) == 0 {
				return exitcode.Errorf(exitcode.NotFound,
					"no active flows found for %s", arg)
			}

			// Hydrate details via strict lookups
			var flows []statesvc.FlowEntry
			subj := broker.ServiceFlowsSubject(cfg.Username)

			for _, id := range flowIDs {
				query := statesvc.FlowsQuery{}
				if strings.HasPrefix(id, "fl_") {
					query.FlowID = id
				} else if strings.HasPrefix(id, "ws_") {
					query.WorkspaceID = id
				}
				queryData, _ := json.Marshal(query)

				resp, err := nc.Request(subj, queryData, 2*time.Second)
				if err != nil {
					if err == nats.ErrTimeout {
						return exitcode.Errorf(exitcode.Error, "daemon not responding (is it running?)")
					}
					return exitcode.Errorf(exitcode.Error, "query flow %s: %v", id, err)
				}
				var result statesvc.FlowsResult
				if err := json.Unmarshal(resp.Data, &result); err != nil {
					return exitcode.Errorf(exitcode.Error, "parse response for %s: %v", id, err)
				}
				flows = append(flows, result.Flows...)
			}

			if len(flows) == 0 {
				return exitcode.Errorf(exitcode.NotFound,
					"no active flows found for %s", arg)
			}

			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				if len(flows) == 1 {
					return enc.Encode(flows[0])
				}
				return enc.Encode(flows)
			}

			gracePeriod := cfg.Reaping.GracePeriod.Duration
			now := time.Now()
			w := tabwriter.NewWriter(cmd.OutOrStdout(),
				0, 0, 2, ' ', 0)

			for i, f := range flows {
				if i > 0 {
					fmt.Fprintf(w, "---\n")
				}
				ttl := f.LastActivityTimestamp.Add(gracePeriod).Sub(now)

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
			}

			w.Flush()
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
