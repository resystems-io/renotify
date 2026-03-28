package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newRevokeCmd(app *App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke the active mobile pairing token",
		Long: `Remove the active pairing token and disconnect the mobile
client. For the embedded broker the token is removed from the NATS
auth configuration immediately. For a shared broker the local token
file is deleted but the operator must revoke on the broker side.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = app.Config
			_ = format

			fmt.Fprintln(cmd.ErrOrStderr(), "revoke: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
