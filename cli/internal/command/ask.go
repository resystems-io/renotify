package command

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	natsjs "github.com/nats-io/nats.go/jetstream"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/payload"
)

func newAskCmd(app *App) *cobra.Command {
	var (
		title         string
		message       string
		priority      string
		source        string
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

If --message is not provided, the message is read from stdin.

The daemon is the sole timeout enforcer (R-CLI-17). The CLI does
not run a local timer; it waits for the daemon to publish an
ErrorResponse on timeout. A safety timer (timeout + grace period)
protects against daemon failure.`,
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

			// Validate and convert response types.
			rts := make([]payload.ResponseType, len(responseTypes))
			for i, rt := range responseTypes {
				switch payload.ResponseType(rt) {
				case payload.ResponseBoolean, payload.ResponseChoice, payload.ResponseText:
					rts[i] = payload.ResponseType(rt)
				default:
					return &exitcode.CodedError{
						Code:    exitcode.Usage,
						Message: fmt.Sprintf("invalid response type %q (use boolean, choice, or text)", rt),
					}
				}
			}

			// Validate choice requires actions.
			hasChoice := false
			for _, rt := range rts {
				if rt == payload.ResponseChoice {
					hasChoice = true
					break
				}
			}
			if hasChoice && len(actions) == 0 {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--actions is required when response-types includes choice",
				}
			}

			p := payload.Priority(priority)
			switch p {
			case payload.PriorityLow, payload.PriorityNormal, payload.PriorityHigh:
			default:
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: fmt.Sprintf("invalid priority %q (use low, normal, or high)", priority),
				}
			}

			// Parse timeout.
			cfg := app.Config
			timeoutDur := cfg.Timeout.DefaultAskTimeout.Duration
			if timeout != "" {
				var err error
				timeoutDur, err = time.ParseDuration(timeout)
				if err != nil {
					return &exitcode.CodedError{
						Code:    exitcode.Usage,
						Message: fmt.Sprintf("invalid timeout %q: %v", timeout, err),
					}
				}
			}
			timeoutSec := int(timeoutDur.Seconds())

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

			fc, err := setupFlow(cfg)
			if err != nil {
				return err
			}
			defer fc.close()

			// Create JetStream handle (new API for consumer
			// management).
			js, err := natsjs.New(fc.nc)
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"jetstream: %v", err)
			}

			// Create ephemeral consumers BEFORE publishing the
			// request to avoid a race where the response arrives
			// before the consumer exists.
			respConsumer, err := js.CreateConsumer(
				cmd.Context(), broker.StreamName,
				natsjs.ConsumerConfig{
					FilterSubject: broker.FlowResponseSubject(
						fc.username, fc.flowID),
					AckPolicy:     natsjs.AckExplicitPolicy,
					DeliverPolicy: natsjs.DeliverNewPolicy,
				})
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"create response consumer: %v", err)
			}

			interjConsumer, err := js.CreateConsumer(
				cmd.Context(), broker.StreamName,
				natsjs.ConsumerConfig{
					FilterSubject: broker.FlowInterjectSubject(
						fc.username, fc.flowID),
					AckPolicy:     natsjs.AckExplicitPolicy,
					DeliverPolicy: natsjs.DeliverNewPolicy,
				})
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"create interjection consumer: %v", err)
			}

			// Set up signal handling for Ctrl-C.
			sigCtx, sigStop := signal.NotifyContext(
				cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer sigStop()

			// Start message iterators.
			respMsgs, err := respConsumer.Messages(
				natsjs.PullMaxMessages(1))
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"subscribe response: %v", err)
			}
			defer respMsgs.Stop()

			interjMsgs, err := interjConsumer.Messages(
				natsjs.PullMaxMessages(1))
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"subscribe interjection: %v", err)
			}
			defer interjMsgs.Stop()

			now := time.Now().UTC()

			// Publish flow-active lifecycle event (legacy API
			// for publishing — matches post.go).
			legacyJS, _ := fc.nc.JetStream()

			activeEvent := &payload.FlowLifecycleEvent{
				FlowID:      fc.flowID,
				DaemonID:    fc.daemonID,
				WorkspaceID: fc.workspaceID,
				Status:      payload.FlowActive,
				Metadata:    fc.workspaceMetadata(),
				Timestamp:   now,
			}
			if err := broker.PublishJSON(legacyJS,
				broker.FlowLifecycleSubject(fc.username, fc.flowID),
				fc.flowID, activeEvent,
			); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"publish lifecycle (active): %v", err)
			}

			// Publish notification request.
			req := &payload.NotificationRequest{
				ID:            fc.notificationID,
				FlowID:        fc.flowID,
				DaemonID:      fc.daemonID,
				WorkspaceID:   fc.workspaceID,
				Title:         title,
				Body:          message,
				ResponseTypes: rts,
				Priority:      p,
				Source:        source,
				WorkspaceName: fc.displayName,
				Actions:       actions,
				TimeoutSec:    timeoutSec,
				Timestamp:     now,
			}
			if err := broker.PublishJSON(legacyJS,
				broker.FlowRequestSubject(fc.username, fc.flowID),
				fc.notificationID, req,
			); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"publish notification: %v", err)
			}

			fmt.Fprintf(cmd.ErrOrStderr(),
				"Waiting for response (flow=%s, notification=%s)\n",
				fc.flowID, fc.notificationID)

			// Safety timer: timeout + grace period.
			grace := cfg.Timeout.AskGracePeriod.Duration
			safetyTimer := time.NewTimer(timeoutDur + grace)
			defer safetyTimer.Stop()

			// Channel adapters for the message iterators.
			respCh := make(chan natsjs.Msg, 1)
			interjCh := make(chan natsjs.Msg, 1)
			go pumpMessages(respMsgs, respCh)
			go pumpMessages(interjMsgs, interjCh)

			// publishFailed is a helper for terminal events
			// where the CLI must publish a failed lifecycle.
			publishFailed := func() {
				failedEvent := &payload.FlowLifecycleEvent{
					FlowID:      fc.flowID,
					DaemonID:    fc.daemonID,
					WorkspaceID: fc.workspaceID,
					Status:      payload.FlowFailed,
					Timestamp:   time.Now().UTC(),
				}
				broker.PublishJSON(legacyJS,
					broker.FlowLifecycleSubject(fc.username, fc.flowID),
					fc.flowID+"-failed", failedEvent)
			}

			// Wait loop.
			for {
				select {
				case msg := <-respCh:
					msg.Ack()
					return handleResponse(cmd, legacyJS, fc,
						msg.Data(), title, format)

				case msg := <-interjCh:
					msg.Ack()
					action, err := handleInterjection(
						cmd, msg.Data())
					if err != nil {
						return err
					}
					if action == payload.InterjectionStop {
						publishFailed()
						return exitcode.Errorf(exitcode.Error,
							"flow stopped by user")
					}
					// note: continue waiting

				case <-safetyTimer.C:
					publishFailed()
					return exitcode.Errorf(exitcode.Timeout,
						"timeout after %s waiting for response "+
							"to %q (daemon did not respond)",
						timeoutDur, title)

				case <-sigCtx.Done():
					publishFailed()
					return exitcode.Errorf(exitcode.Error,
						"interrupted")
				}
			}
		},
	}

	cmd.Flags().StringVarP(&title, "title", "t", "",
		"notification title (required)")
	cmd.Flags().StringVarP(&message, "message", "m", "",
		"notification message (reads stdin if omitted)")
	cmd.Flags().StringVarP(&priority, "priority", "p", "normal",
		"low|normal|high")
	cmd.Flags().StringVarP(&source, "source", "s", "",
		"source pipeline identifier")
	cmd.Flags().StringSliceVarP(&actions, "actions", "a", nil,
		"choice labels (required when response-types includes choice)")
	cmd.Flags().StringSliceVarP(&responseTypes, "response-types", "r", nil,
		"accepted response types: boolean, choice, text (required)")
	cmd.Flags().StringVar(&timeout, "timeout", "",
		"override default ask timeout (e.g., 5m, 10m)")
	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}

// pumpMessages reads from a JetStream message iterator and sends
// each message to the channel. Exits when the iterator is stopped
// or the underlying context is cancelled.
func pumpMessages(iter natsjs.MessagesContext, ch chan<- natsjs.Msg) {
	for {
		msg, err := iter.Next()
		if err != nil {
			return
		}
		ch <- msg
	}
}

// handleResponse processes a message from the .response subject,
// discriminating between NotificationResponse and ErrorResponse.
func handleResponse(
	cmd *cobra.Command,
	js nats.JetStreamContext,
	fc *flowContext,
	data []byte,
	title, format string,
) error {
	if isErrorResponse(data) {
		var errResp payload.ErrorResponse
		json.Unmarshal(data, &errResp)
		return handleErrorResponse(cmd, &errResp, title, fc)
	}

	// It's a NotificationResponse.
	var resp payload.NotificationResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return exitcode.Errorf(exitcode.Error,
			"unmarshal response: %v", err)
	}

	// Publish flow-completed lifecycle event.
	completedEvent := &payload.FlowLifecycleEvent{
		FlowID:      fc.flowID,
		DaemonID:    fc.daemonID,
		WorkspaceID: fc.workspaceID,
		Status:      payload.FlowCompleted,
		Timestamp:   time.Now().UTC(),
	}
	broker.PublishJSON(js,
		broker.FlowLifecycleSubject(fc.username, fc.flowID),
		fc.flowID+"-completed", completedEvent)

	// Output the response.
	if format == "text" {
		formatResponseText(cmd.OutOrStdout(), &resp)
	} else {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetEscapeHTML(false)
		enc.Encode(resp)
	}
	return nil
}

// handleErrorResponse maps an ErrorResponse to the appropriate
// exit code.
func handleErrorResponse(
	cmd *cobra.Command,
	errResp *payload.ErrorResponse,
	title string,
	fc *flowContext,
) error {
	switch errResp.Code {
	case "timeout":
		return exitcode.Errorf(exitcode.Timeout,
			"timeout waiting for response to %q", title)
	case "rate_limited":
		return exitcode.Errorf(exitcode.RateLimited,
			"rate limit exceeded: %s", errResp.Message)
	case "unroutable":
		return exitcode.Errorf(exitcode.Unroutable,
			"no mobile client connected")
	default:
		return exitcode.Errorf(exitcode.Error,
			"daemon error: %s", errResp.Message)
	}
}

// handleInterjection processes a message from the .interject
// subject. Returns the action so the caller can decide whether
// to exit (stop) or continue waiting (note).
func handleInterjection(
	cmd *cobra.Command,
	data []byte,
) (payload.InterjectionAction, error) {
	var interj payload.InterjectionCommand
	if err := json.Unmarshal(data, &interj); err != nil {
		return "", exitcode.Errorf(exitcode.Error,
			"unmarshal interjection: %v", err)
	}

	switch interj.Action {
	case payload.InterjectionStop:
		return payload.InterjectionStop, nil
	case payload.InterjectionNote:
		if interj.Context != "" {
			fmt.Fprintf(cmd.ErrOrStderr(),
				"Note: %s\n", interj.Context)
		}
		return payload.InterjectionNote, nil
	default:
		return interj.Action, nil
	}
}

// formatResponseText writes a human-readable response to w.
func formatResponseText(w io.Writer, r *payload.NotificationResponse) {
	if r.Accepted != nil {
		if *r.Accepted {
			fmt.Fprintln(w, "Response: Yes")
		} else {
			fmt.Fprintln(w, "Response: No")
		}
	}
	if r.Action != "" {
		fmt.Fprintf(w, "Response: %s\n", r.Action)
	}
	if r.Text != "" {
		fmt.Fprintf(w, "Comment:  %s\n", r.Text)
	}
}
