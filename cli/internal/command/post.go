package command

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/exitcode"
)

func newPostCmd(app *App) *cobra.Command {
	var (
		title    string
		body     string
		priority string
		source   string
		format   string
	)

	cmd := &cobra.Command{
		Use:   "post",
		Short: "Send a fire-and-forget notification",
		Long: `Publish a one-way notification to the mobile app. The command
exits immediately after publishing; no response is expected.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--title is required",
				}
			}

			_ = app.Config
			_ = format

			fmt.Fprintln(cmd.ErrOrStderr(), "post: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "notification title (required)")
	cmd.Flags().StringVar(&body, "body", "", "notification body")
	cmd.Flags().StringVar(&priority, "priority", "normal", "low|normal|high")
	cmd.Flags().StringVar(&source, "source", "", "source pipeline identifier")
	cmd.Flags().StringVar(&format, "format", "text", "output format: json|text")

	return cmd
}
