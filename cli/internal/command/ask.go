package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/exitcode"
)

func newAskCmd(app *App) *cobra.Command {
	var (
		title         string
		body          string
		priority      string
		actions       []string
		responseTypes []string
		timeout       string
		format        string
	)

	cmd := &cobra.Command{
		Use:   "ask",
		Short: "Send a blocking notification and wait for a response",
		Long: `Publish a notification that requires a human decision and block
until a response is received or the timeout expires. The response
is printed to stdout as JSON (default) or formatted text.

The daemon is the sole timeout enforcer (R-CLI-17). The CLI does
not run a local timer; it waits for the daemon to publish an
ErrorResponse on timeout.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--title is required",
				}
			}
			if len(responseTypes) == 0 {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--response-types is required (boolean, choice, text)",
				}
			}

			_ = app.Config
			_ = timeout
			_ = format
			_ = actions

			fmt.Fprintln(cmd.ErrOrStderr(), "ask: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "notification title (required)")
	cmd.Flags().StringVar(&body, "body", "", "notification body")
	cmd.Flags().StringVar(&priority, "priority", "normal", "low|normal|high")
	cmd.Flags().StringSliceVar(&actions, "actions", nil,
		"choice labels (required when response-types includes choice)")
	cmd.Flags().StringSliceVar(&responseTypes, "response-types", nil,
		"accepted response types: boolean, choice, text (required)")
	cmd.Flags().StringVar(&timeout, "timeout", "",
		"override default ask timeout (e.g., 5m, 10m)")
	cmd.Flags().StringVar(&format, "format", "json", "output format: json|text")

	return cmd
}
