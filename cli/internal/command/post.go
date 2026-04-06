// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

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

func newPostCmd(app *App) *cobra.Command {
	var (
		title    string
		message  string
		priority string
		source   string
		format   string
	)

	cmd := &cobra.Command{
		Use:   "post",
		Short: "Send a fire-and-forget notification",
		Long: `Publish a one-way notification to the mobile app. The command
exits immediately after publishing; no response is expected.

If --message is not provided, the message is read from stdin.
This allows piping output from other commands:

  echo "All 42 tests passed" | renotify post -t "Build done"

The daemon must be running (renotify daemon start) before
posting. The notification is buffered in JetStream for up to
30 minutes if the mobile app is temporarily disconnected.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if title == "" {
				return &exitcode.CodedError{
					Code:    exitcode.Usage,
					Message: "--title is required",
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

			fc, err := setupFlow(app.Config)
			if err != nil {
				return err
			}
			defer fc.close()

			js, err := fc.nc.JetStream()
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"jetstream: %v", err)
			}

			now := time.Now().UTC()

			// Publish flow-active lifecycle event.
			activeEvent := &payload.FlowLifecycleEvent{
				FlowID:      fc.flowID,
				DaemonID:    fc.daemonID,
				WorkspaceID: fc.workspaceID,
				Status:      payload.FlowActive,
				Metadata:    fc.workspaceMetadata(),
				Timestamp:   now,
			}
			if err := broker.PublishJSON(js,
				broker.FlowLifecycleSubject(fc.username, fc.flowID),
				fc.flowID, activeEvent,
			); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"publish lifecycle (active): %v", err)
			}

			// Publish notification request.
			req := &payload.NotificationRequest{
				ID:          fc.notificationID,
				FlowID:      fc.flowID,
				DaemonID:    fc.daemonID,
				WorkspaceID: fc.workspaceID,
				Title:       title,
				Body:        message,
				ResponseTypes: []payload.ResponseType{
					payload.ResponseNone,
				},
				Priority:      p,
				Source:        source,
				WorkspaceName: fc.displayName,
				Timestamp:     now,
			}
			if err := broker.PublishJSON(js,
				broker.FlowRequestSubject(fc.username, fc.flowID),
				fc.notificationID, req,
			); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"publish notification: %v", err)
			}

			// Publish flow-completed lifecycle event.
			completedEvent := &payload.FlowLifecycleEvent{
				FlowID:      fc.flowID,
				DaemonID:    fc.daemonID,
				WorkspaceID: fc.workspaceID,
				Status:      payload.FlowCompleted,
				Timestamp:   now,
			}
			if err := broker.PublishJSON(js,
				broker.FlowLifecycleSubject(fc.username, fc.flowID),
				fc.flowID+"-completed", completedEvent,
			); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"publish lifecycle (completed): %v", err)
			}

			// Output.
			if format == "json" {
				out := struct {
					Status         string `json:"status"`
					NotificationID string `json:"notification_id"`
				}{
					Status:         "sent",
					NotificationID: fc.notificationID,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetEscapeHTML(false)
				enc.Encode(out)
			}

			return nil
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
	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
