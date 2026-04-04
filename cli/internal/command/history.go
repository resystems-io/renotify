package command

import (
	"encoding/json"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/payload"
	"go.resystems.io/renotify/internal/statesvc"
)

func newHistoryCmd(app *App) *cobra.Command {
	var (
		workspaceID string
		flowID      string
		since       string
		until       string
		limit       int
		offset      int
		format      string
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Query the notification history ledger",
		Long: `Retrieve historical notification records from the daemon's
SQLite ledger. Results include the original notification request
paired with its response (if one was received). Supports filtering
by workspace, flow, time range, and pagination via limit/offset.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config

			nc, err := broker.ConnectCLI(cfg)
			if err != nil {
				return exitcode.Errorf(exitcode.Error, "%v", err)
			}
			defer nc.Drain()

			// Build query request.
			req := statesvc.HistoryQueryRequest{
				WorkspaceID: workspaceID,
				FlowID:      flowID,
				Limit:       limit,
				Offset:      offset,
			}

			if since != "" {
				t, err := time.Parse(time.RFC3339, since)
				if err != nil {
					return exitcode.Errorf(exitcode.Usage,
						"invalid --since: %v", err)
				}
				req.Since = &t
			}
			if until != "" {
				t, err := time.Parse(time.RFC3339, until)
				if err != nil {
					return exitcode.Errorf(exitcode.Usage,
						"invalid --until: %v", err)
				}
				req.Until = &t
			}

			reqData, _ := json.Marshal(req)

			// Request history from daemon.
			subject := broker.ServiceHistorySubject(cfg.Username)
			resp, err := nc.Request(subject, reqData,
				2*time.Second)
			if err != nil {
				if err == nats.ErrTimeout {
					return exitcode.Errorf(exitcode.Error,
						"daemon not responding (is it running?)")
				}
				return exitcode.Errorf(exitcode.Error,
					"query history: %v", err)
			}

			var result statesvc.HistoryQueryResult
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

			return formatHistoryText(cmd, &result)
		},
	}

	cmd.Flags().StringVarP(&workspaceID, "workspace", "w", "",
		"filter by workspace ID")
	cmd.Flags().StringVarP(&flowID, "flow-id", "f", "",
		"filter by flow")
	cmd.Flags().StringVar(&since, "since", "",
		"include records since, RFC 3339 (e.g. 2026-04-01T00:00:00Z)")
	cmd.Flags().StringVar(&until, "until", "",
		"include records until, RFC 3339 (e.g. 2026-04-02T23:59:59Z)")
	cmd.Flags().IntVar(&limit, "limit", 0,
		"maximum records to return")
	cmd.Flags().IntVar(&offset, "offset", 0,
		"skip first N records (pagination)")
	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}

// formatHistoryText renders history records in a human-readable
// tabular format.
func formatHistoryText(
	cmd *cobra.Command,
	result *statesvc.HistoryQueryResult,
) error {
	out := cmd.OutOrStdout()

	if len(result.Records) == 0 {
		fmt.Fprintln(out, "No history records.")
		return nil
	}

	fmt.Fprintf(out, "Showing %d of %d records\n\n",
		len(result.Records), result.Total)

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tTIMESTAMP\tTITLE\tPRIORITY\tRESPONSE")

	for _, rec := range result.Records {
		r := rec.Request
		respStr := formatResponse(rec.Response)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.ID,
			r.Timestamp.Format("2006-01-02 15:04:05"),
			truncate(r.Title, 40),
			r.Priority,
			respStr,
		)
	}

	w.Flush()
	return nil
}

// formatResponse summarises a NotificationResponse for the text
// format. Returns "-" for unanswered notifications.
func formatResponse(resp *payload.NotificationResponse) string {
	if resp == nil {
		return "-"
	}
	if resp.Accepted != nil {
		if *resp.Accepted {
			return "accepted"
		}
		return "denied"
	}
	if resp.Action != "" {
		return resp.Action
	}
	if resp.Text != "" {
		return truncate(resp.Text, 30)
	}
	return "-"
}
