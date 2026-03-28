package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/exitcode"
)

func newDaemonCmd(app *App) *cobra.Command {
	var (
		foreground bool
		username   string
	)

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the Renotify daemon",
		Long: `Start the daemon process that runs the embedded NATS broker,
MCP server, and background services. Runs in the background by
default; use --foreground to run in the foreground with logs to
stderr.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config

			// Apply flag overrides to config.
			if cmd.Flags().Changed("foreground") {
				cfg.Daemon.Foreground = foreground
			}
			if cmd.Flags().Changed("username") {
				cfg.Username = username
			}

			// Validate after flag overrides.
			if err := cfg.Validate(); err != nil {
				return &exitcode.CodedError{
					Code:    exitcode.Error,
					Message: fmt.Sprintf("config: %v", err),
				}
			}

			fmt.Fprintln(cmd.ErrOrStderr(), "daemon: not yet implemented")
			return nil
		},
	}

	cmd.Flags().BoolVar(&foreground, "foreground", false,
		"run in foreground (logs to stderr)")
	cmd.Flags().StringVar(&username, "username", "",
		"NATS auth identity (required)")

	return cmd
}
