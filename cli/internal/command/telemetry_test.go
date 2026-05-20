// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestTelemetryList_TextAndJSON(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	// Provision the RENOTIFY_TELEMETRY stream in our test server.
	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "RENOTIFY_TELEMETRY",
		Subjects: []string{"resystems.renotify.*.device.*.telemetry.>"},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Publish two mock crash reports to NATS.
	report1 := telemetryReport{
		ReportID:     "ntf_crash001",
		DeviceID:     "mb_testdevice",
		Timestamp:    "2026-05-17T12:00:00Z",
		IncidentType: "managed_crash",
		IncidentDetails: telemetryDetails{
			ExceptionType: "java.lang.NullPointerException",
			Message:       "null pointer exception message",
			StackTrace:    "stack trace line 1\nline 2",
		},
	}
	report2 := telemetryReport{
		ReportID:     "ntf_kill002",
		DeviceID:     "mb_testdevice",
		Timestamp:    "2026-05-17T12:05:00Z",
		IncidentType: "unmanaged_kill",
		IncidentDetails: telemetryDetails{
			ExceptionType: "UnmanagedKill",
			Message:       "Process terminated by system",
			StackTrace:    "",
		},
	}

	r1Bytes, _ := json.Marshal(report1)
	r2Bytes, _ := json.Marshal(report2)

	_, err = js.Publish("resystems.renotify.testuser.device.mb_testdevice.telemetry.crash", r1Bytes)
	if err != nil {
		t.Fatal(err)
	}
	_, err = js.Publish("resystems.renotify.testuser.device.mb_testdevice.telemetry.crash", r2Bytes)
	if err != nil {
		t.Fatal(err)
	}

	// Test Text output format.
	stdoutText, _, err := executeCommand("telemetry", "list")
	if err != nil {
		t.Fatalf("telemetry list failed: %v", err)
	}

	if !strings.Contains(stdoutText, "java.lang.NullPointerException") {
		t.Errorf("text output missing exception class, got: %q", stdoutText)
	}
	if !strings.Contains(stdoutText, "UnmanagedKill") {
		t.Errorf("text output missing unmanaged kill status, got: %q", stdoutText)
	}
	if !strings.Contains(stdoutText, "mb_testdevice") {
		t.Errorf("text output missing device ID, got: %q", stdoutText)
	}
	if !strings.Contains(stdoutText, "ntf_crash001") {
		t.Errorf("text output missing report ID ntf_crash001, got: %q", stdoutText)
	}
	if !strings.Contains(stdoutText, "ntf_kill002") {
		t.Errorf("text output missing report ID ntf_kill002, got: %q", stdoutText)
	}

	// Test JSON output format.
	stdoutJSON, _, err := executeCommand("telemetry", "list", "--format", "json")
	if err != nil {
		t.Fatalf("telemetry list --format json failed: %v", err)
	}

	var parsedReports []telemetryReport
	if err := json.Unmarshal([]byte(stdoutJSON), &parsedReports); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdoutJSON)
	}

	if len(parsedReports) != 2 {
		t.Errorf("expected 2 reports, got %d", len(parsedReports))
	}
	if parsedReports[0].ReportID != "ntf_crash001" {
		t.Errorf("expected parsed[0] report ID 'ntf_crash001', got %q", parsedReports[0].ReportID)
	}
	if parsedReports[1].IncidentDetails.ExceptionType != "UnmanagedKill" {
		t.Errorf("expected parsed[1] exception type 'UnmanagedKill', got %q", parsedReports[1].IncidentDetails.ExceptionType)
	}
}

func TestTelemetryFetch(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	_, err = js.AddStream(&nats.StreamConfig{
		Name:     "RENOTIFY_TELEMETRY",
		Subjects: []string{"resystems.renotify.*.device.*.telemetry.>"},
		Storage:  nats.MemoryStorage,
	})
	if err != nil {
		t.Fatal(err)
	}

	report := telemetryReport{
		ReportID:     "ntf_crash999",
		DeviceID:     "mb_testdevice",
		Timestamp:    "2026-05-17T12:00:00Z",
		IncidentType: "managed_crash",
		IncidentDetails: telemetryDetails{
			ExceptionType: "java.lang.IllegalArgumentException",
			Message:       "invalid argument",
			StackTrace:    "",
		},
	}
	rBytes, _ := json.Marshal(report)

	_, err = js.Publish("resystems.renotify.testuser.device.mb_testdevice.telemetry.crash", rBytes)
	if err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	stdout, _, err := executeCommand("telemetry", "fetch", outDir)
	if err != nil {
		t.Fatalf("telemetry fetch failed: %v", err)
	}

	if !strings.Contains(stdout, "Successfully fetched 1 incident report(s)") {
		t.Errorf("unexpected fetch message: %q", stdout)
	}

	// Verify the file was written to disk.
	filePath := filepath.Join(outDir, "ntf_crash999.json")
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	var writtenReport telemetryReport
	if err := json.Unmarshal(fileBytes, &writtenReport); err != nil {
		t.Fatalf("malformed written JSON: %v", err)
	}

	if writtenReport.ReportID != "ntf_crash999" || writtenReport.IncidentDetails.ExceptionType != "java.lang.IllegalArgumentException" {
		t.Errorf("written file contents mismatch: %+v", writtenReport)
	}
}
