// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	natsjs "github.com/nats-io/nats.go/jetstream"
	"github.com/spf13/cobra"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/exitcode"
)

type telemetryReport struct {
	ReportID        string           `json:"report_id"`
	DeviceID        string           `json:"device_id"`
	Timestamp       string           `json:"timestamp"`
	IncidentType    string           `json:"incident_type"`
	IncidentDetails telemetryDetails `json:"incident_details"`
}

type telemetryDetails struct {
	ExceptionType string `json:"exception_type"`
	Message       string `json:"message"`
	StackTrace    string `json:"stack_trace"`
}

func newTelemetryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Manage and query device diagnostic telemetry",
		Long: `Query and retrieve diagnostic incident reports from paired mobile
devices. Telemetry is collected in a dedicated, file-backed stream
isolated from operational databases.`,
	}

	cmd.AddCommand(
		newTelemetryListCmd(app),
		newTelemetryFetchCmd(app),
	)

	return cmd
}

func newTelemetryListCmd(app *App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent diagnostic incident reports",
		Long: `Connects to the daemon's embedded broker, queries the telemetry
stream, and outputs a summary of all captured crashes and kills.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTelemetryList(app, cmd, format)
		},
		SilenceUsage: true,
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: json|text")
	return cmd
}

func newTelemetryFetchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fetch [output-directory]",
		Short: "Download all telemetry JSON payloads to a directory",
		Long: `Downloads all pending and historical incident report JSON payloads
from the telemetry stream and writes them sequentially to a local directory.
If no directory is specified, it defaults to the current directory.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			return runTelemetryFetch(app, cmd, dir)
		},
		SilenceUsage: true,
	}

	return cmd
}

func runTelemetryList(app *App, cmd *cobra.Command, format string) error {
	cfg := app.Config
	nc, err := broker.ConnectCLI(cfg)
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"cannot connect to daemon: %v\n"+
				"Is the daemon running? Check with: renotify daemon status",
			err)
	}
	defer nc.Close()

	js, err := natsjs.New(nc)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to create jetstream client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := js.Stream(ctx, broker.TelemetryStreamName)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to get telemetry stream %s: %v", broker.TelemetryStreamName, err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to get stream info: %v", err)
	}

	if info.State.Msgs == 0 {
		if format == "json" {
			fmt.Fprintln(cmd.OutOrStdout(), "[]")
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "No incident reports found.")
		return nil
	}

	// Create an ephemeral consumer to pull all messages in the stream
	cons, err := stream.CreateOrUpdateConsumer(ctx, natsjs.ConsumerConfig{
		DeliverPolicy: natsjs.DeliverAllPolicy,
		AckPolicy:     natsjs.AckNonePolicy,
	})
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to create consumer: %v", err)
	}

	// Fetch matches the exact current stream count (up to 100 limit).
	// Because request matches broker availability, it returns immediately (0ms).
	fetchLimit := int(info.State.Msgs)
	if fetchLimit > 100 {
		fetchLimit = 100
	}

	var reports []telemetryReport
	lastSeq := info.State.LastSeq

	// Fetch loop, breaking instantly when sequence reaches LastSeq to prevent empty NATS timeouts.
	for {
		batch, err := cons.Fetch(fetchLimit, natsjs.FetchMaxWait(100*time.Millisecond))
		if err != nil {
			break
		}
		count := 0
		reachedEnd := false
		for msg := range batch.Messages() {
			count++
			var report telemetryReport
			if err := json.Unmarshal(msg.Data(), &report); err == nil {
				reports = append(reports, report)
			}
			meta, err := msg.Metadata()
			if err == nil && meta.Sequence.Stream >= lastSeq {
				reachedEnd = true
			}
		}
		if count == 0 || reachedEnd || len(reports) >= fetchLimit {
			break
		}
	}

	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(reports)
	}

	if len(reports) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "No incident reports found.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "REPORT ID\tTIMESTAMP\tDEVICE ID\tTYPE\tEXCEPTION\n")
	for _, r := range reports {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.ReportID,
			r.Timestamp,
			r.DeviceID,
			r.IncidentType,
			r.IncidentDetails.ExceptionType,
		)
	}
	w.Flush()
	return nil
}

func runTelemetryFetch(app *App, cmd *cobra.Command, dir string) error {
	cfg := app.Config
	nc, err := broker.ConnectCLI(cfg)
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"cannot connect to daemon: %v\n"+
				"Is the daemon running? Check with: renotify daemon status",
			err)
	}
	defer nc.Close()

	// Ensure output directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to create output directory %s: %v", dir, err)
	}

	js, err := natsjs.New(nc)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to create jetstream client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := js.Stream(ctx, broker.TelemetryStreamName)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to get telemetry stream %s: %v", broker.TelemetryStreamName, err)
	}

	info, err := stream.Info(ctx)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to get stream info: %v", err)
	}

	if info.State.Msgs == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Successfully fetched 0 incident report(s) into %s\n", dir)
		return nil
	}

	cons, err := stream.CreateOrUpdateConsumer(ctx, natsjs.ConsumerConfig{
		DeliverPolicy: natsjs.DeliverAllPolicy,
		AckPolicy:     natsjs.AckNonePolicy,
	})
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "failed to create consumer: %v", err)
	}

	fetchLimit := int(info.State.Msgs)
	if fetchLimit > 100 {
		fetchLimit = 100
	}

	count := 0
	lastSeq := info.State.LastSeq

	for {
		batch, err := cons.Fetch(fetchLimit, natsjs.FetchMaxWait(100*time.Millisecond))
		if err != nil {
			break
		}
		batchCount := 0
		reachedEnd := false
		for msg := range batch.Messages() {
			batchCount++
			var report telemetryReport
			if err := json.Unmarshal(msg.Data(), &report); err == nil {
				fileName := report.ReportID
				if fileName == "" {
					fileName = fmt.Sprintf("report-%d", count)
				}
				filePath := filepath.Join(dir, fmt.Sprintf("%s.json", fileName))

				if err := os.WriteFile(filePath, msg.Data(), 0644); err != nil {
					return exitcode.Errorf(exitcode.Error, "failed to write report to %s: %v", filePath, err)
				}
				count++
			}
			meta, err := msg.Metadata()
			if err == nil && meta.Sequence.Stream >= lastSeq {
				reachedEnd = true
			}
		}
		if batchCount == 0 || reachedEnd || count >= fetchLimit {
			break
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Successfully fetched %d incident report(s) into %s\n", count, dir)
	return nil
}
