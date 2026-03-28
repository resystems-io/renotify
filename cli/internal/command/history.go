package command

import (
	"fmt"

	"github.com/spf13/cobra"
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
			_ = app.Config
			_ = workspaceID
			_ = flowID
			_ = since
			_ = until
			_ = limit
			_ = offset
			_ = format

			fmt.Fprintln(cmd.ErrOrStderr(), "history: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&workspaceID, "workspace-id", "", "filter by workspace")
	cmd.Flags().StringVar(&flowID, "flow-id", "", "filter by flow")
	cmd.Flags().StringVar(&since, "since", "", "include records since (RFC 3339)")
	cmd.Flags().StringVar(&until, "until", "", "include records until (RFC 3339)")
	cmd.Flags().IntVar(&limit, "limit", 0, "maximum records to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "skip first N records (pagination)")
	cmd.Flags().StringVar(&format, "format", "json", "output format: json|text")

	return cmd
}
