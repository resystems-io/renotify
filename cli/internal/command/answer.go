package command

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/payload"
)

func newAnswerCmd(app *App) *cobra.Command {
	var (
		flowID    string
		requestID string
		accepted  bool
		rejected  bool
		action    string
		message   string
		format    string
	)

	cmd := &cobra.Command{
		Use:   "answer",
		Short: "Publish a response to a waiting ask notification",
		Long: `Publish a NotificationResponse to unblock a waiting
'renotify ask' command. The flow-id and request-id are obtained
from the ask command's stderr output.

If --message is not provided, the message is read from stdin
when piped.

Examples:

  renotify answer -f fl_... -n ntf_... --accepted
  renotify answer -f fl_... -n ntf_... --rejected -m "Not ready"
  renotify answer -f fl_... -n ntf_... -a "Approve"
  echo "Reason" | renotify answer -f fl_... -n ntf_... --rejected`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flowID == "" {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--flow-id is required",
				}
			}
			if requestID == "" {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--request-id is required",
				}
			}
			if accepted && rejected {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--accepted and --rejected are mutually exclusive",
				}
			}

			// Read message from stdin if not provided via flag.
			if !cmd.Flags().Changed("message") {
				if info, _ := os.Stdin.Stat(); info.Mode()&os.ModeCharDevice == 0 {
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return exitcode.Errorf(exitcode.Error,
							"read stdin: %v", err)
					}
					message = strings.TrimRight(string(data), "\n")
				}
			}

			// Build the response.
			resp := &payload.NotificationResponse{
				RequestID: requestID,
				Timestamp: time.Now().UTC(),
			}
			if accepted {
				b := true
				resp.Accepted = &b
			} else if rejected {
				b := false
				resp.Accepted = &b
			}
			if action != "" {
				resp.Action = action
			}
			if message != "" {
				resp.Text = message
			}

			// Connect and publish.
			cfg := app.Config
			nc, err := broker.ConnectCLI(cfg)
			if err != nil {
				return exitcode.Errorf(exitcode.Error, "%v", err)
			}
			defer nc.Drain()

			js, err := nc.JetStream()
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"jetstream: %v", err)
			}

			subject := broker.FlowResponseSubject(
				cfg.Username, flowID)
			if err := broker.PublishJSON(js, subject,
				requestID+"-response", resp,
			); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"publish response: %v", err)
			}

			// Output.
			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				enc.Encode(resp)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&flowID, "flow-id", "f", "",
		"flow identifier (required)")
	cmd.Flags().StringVarP(&requestID, "request-id", "n", "",
		"notification identifier to respond to (required)")
	cmd.Flags().BoolVar(&accepted, "accepted", false,
		"boolean response: accepted")
	cmd.Flags().BoolVar(&rejected, "rejected", false,
		"boolean response: rejected")
	cmd.Flags().StringVarP(&action, "action", "a", "",
		"choice response: selected action label")
	cmd.Flags().StringVarP(&message, "message", "m", "",
		"free-form message / comment (reads stdin if omitted)")
	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
