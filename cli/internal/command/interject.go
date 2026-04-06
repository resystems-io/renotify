package command

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/exitcode"
	"go.resystems.io/renotify/cli/internal/payload"
)

func newInterjectCmd(app *App) *cobra.Command {
	var (
		flowID  string
		message string
		format  string
	)

	cmd := &cobra.Command{
		Use:   "interject [stop|pause|note]",
		Short: "Send a control signal to a running flow",
		Long: `Publish an InterjectionCommand to a running flow. The action
is a positional argument: stop, pause, or note.

A 'stop' signal requests graceful termination of the flow. A
'note' signal delivers informational context without altering
execution. A 'pause' signal requests the pipeline to suspend.

The flow-id is obtained from the ask command's stderr output.

If --message is not provided, the message is read from stdin
when piped.

Examples:

  renotify interject -f fl_... stop
  renotify interject -f fl_... note -m "Staging is down"
  echo "Budget exceeded" | renotify interject -f fl_... stop`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flowID == "" {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--flow-id is required",
				}
			}

			// Validate action.
			action := payload.InterjectionAction(args[0])
			switch action {
			case payload.InterjectionStop, payload.InterjectionPause, payload.InterjectionNote:
			default:
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: fmt.Sprintf("invalid action %q (use stop, pause, or note)", args[0]),
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

			// Build the interjection.
			interj := &payload.InterjectionCommand{
				FlowID:    flowID,
				Action:    action,
				Context:   message,
				Timestamp: time.Now().UTC(),
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

			// Dedup ID includes timestamp to distinguish
			// successive interjections while allowing exact
			// retries within the 2-minute window.
			msgID := fmt.Sprintf("%s-interject-%s-%d",
				flowID, action, interj.Timestamp.UnixMilli())

			subject := broker.FlowInterjectSubject(
				cfg.Username, flowID)
			if err := broker.PublishJSON(js, subject, msgID, interj); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"publish interjection: %v", err)
			}

			// Output.
			if format == "json" {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				enc.Encode(interj)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&flowID, "flow-id", "f", "",
		"flow identifier (required)")
	cmd.Flags().StringVarP(&message, "message", "m", "",
		"free-form message (reads stdin if omitted)")
	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
