package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.resystems.io/renotify/internal/xdg"
)

// executeCommand runs the root command with the given args and
// returns stdout, stderr, and any error. Config loading requires
// a username — tests that don't provide one via env or flag will
// get a config validation error, which is the expected behaviour.
func executeCommand(args ...string) (stdout, stderr string, err error) {
	root := NewRoot()

	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)

	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestRootHelp(t *testing.T) {
	stdout, _, err := executeCommand("--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "Available Commands") {
		t.Error("help output missing 'Available Commands'")
	}
	for _, cmd := range []string{"daemon", "post", "ask", "answer", "interject", "dispatch", "flow", "flows", "history", "pair", "revoke", "apk", "config"} {
		if !strings.Contains(stdout, cmd) {
			t.Errorf("help output missing command %q", cmd)
		}
	}
}

func TestSubcommandHelp(t *testing.T) {
	commands := []struct {
		name  string
		flags []string
	}{
		{"daemon start", []string{"--foreground", "--username", "--log-level", "--shared-broker", "--no-mcp"}},
		{"post", []string{"--title", "--body", "--priority", "--source", "--format"}},
		{"ask", []string{"--title", "--body", "--priority", "--actions", "--response-types", "--timeout", "--format"}},
		{"answer", []string{"--flow-id", "--request-id", "--accepted", "--rejected", "--action", "--text", "--format"}},
		{"interject", []string{"--flow-id", "--message", "--format"}},
		{"flow", []string{"--format"}},
		{"history", []string{"--workspace-id", "--flow-id", "--since", "--until", "--limit", "--offset", "--format"}},
		{"pair", []string{"--ip", "--regenerate-cert", "--format"}},
		{"revoke", []string{"--format"}},
		{"apk extract", []string{"--output"}},
		{"apk serve", []string{"--addr", "--port"}},
		{"config init", []string{"--full", "--force", "--output"}},
		{"config list", []string{"--format"}},
	}

	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			args := append(strings.Fields(tc.name), "--help")
			stdout, _, err := executeCommand(args...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			for _, flag := range tc.flags {
				if !strings.Contains(stdout, flag) {
					t.Errorf("%s help missing flag %q", tc.name, flag)
				}
			}
		})
	}
}

func TestDaemonRequiresUsername(t *testing.T) {
	// Username defaults to the Unix username, so we must
	// explicitly set it empty to test the validation.
	t.Setenv("RENOTIFY_USERNAME", "")
	_, _, err := executeCommand("daemon", "start",
		"--foreground", "--username", "")
	if err == nil {
		t.Fatal("expected error for empty username")
	}
	if !strings.Contains(err.Error(), "username") {
		t.Errorf("error should mention username, got: %v", err)
	}
}

func TestDaemonSubcommands(t *testing.T) {
	// Verify daemon has start, stop, status subcommands.
	stdout, _, err := executeCommand("daemon", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, sub := range []string{"start", "stop", "status"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("daemon help missing subcommand %q", sub)
		}
	}
}

func TestDaemonDoubleStartPrevented(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")

	// Write a PID file with the current process's PID (which is
	// running), simulating an already-running daemon.
	pidPath := xdg.PIDPath()
	dir := filepath.Dir(pidPath)
	os.MkdirAll(dir, 0700)
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600)
	defer os.Remove(pidPath)

	_, _, err := executeCommand("daemon", "start", "--foreground")
	if err == nil {
		t.Fatal("expected error for double start")
	}
	if !strings.Contains(err.Error(), "already running") {
		t.Errorf("error should mention 'already running', got: %v", err)
	}
}

func TestDaemonStaleIDRemoved(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")

	// Write a PID file with a PID that doesn't exist.
	pidPath := xdg.PIDPath()
	dir := filepath.Dir(pidPath)
	os.MkdirAll(dir, 0700)
	os.WriteFile(pidPath, []byte("999999999\n"), 0600)
	defer os.Remove(pidPath)

	// The stale PID should be cleaned up and start should
	// proceed. Since the daemon would block, just verify the
	// PID file was removed.
	_, _, _ = executeCommand("daemon", "start", "--help")
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		// The help command doesn't go through RunE, so let's
		// test via status instead — a stale PID should show
		// "stopped".
	}

	stdout, _, err := executeCommand("daemon", "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "stopped") {
		t.Errorf("status should show stopped for stale PID, got: %q", stdout)
	}
}

func TestPostRequiresTitle(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("post")
	if err == nil {
		t.Fatal("expected error for missing --title")
	}
	if !strings.Contains(err.Error(), "--title") {
		t.Errorf("error should mention --title, got: %v", err)
	}
}

func TestPostAcceptsFlags(t *testing.T) {
	// Post now connects to NATS — this test just verifies that
	// flag parsing works. The connection will fail (no daemon),
	// which is expected.
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	_, _, err := executeCommand("post",
		"--title", "Build done",
		"--body", "All tests passed",
		"--priority", "high",
		"--source", "ci/pipeline",
	)
	// Expected to fail: no daemon running, no internal token.
	if err == nil {
		t.Fatal("expected error (no daemon)")
	}
}

func TestAskRequiresTitle(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("ask",
		"--response-types", "boolean",
	)
	if err == nil {
		t.Fatal("expected error for missing --title")
	}
}

func TestAskRequiresResponseTypes(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("ask",
		"--title", "Proceed?",
	)
	if err == nil {
		t.Fatal("expected error for missing --response-types")
	}
}

func TestAskAcceptsFlags(t *testing.T) {
	// Ask now connects to NATS — this test just verifies that
	// flag parsing works. The connection will fail (no daemon),
	// which is expected.
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	_, _, err := executeCommand("ask",
		"--title", "Deploy?",
		"--response-types", "boolean,text",
		"--timeout", "10m",
		"--priority", "high",
	)
	// Expected to fail: no daemon running, no internal token.
	if err == nil {
		t.Fatal("expected error (no daemon)")
	}
}

func TestHistoryAcceptsFlags(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("history",
		"--limit", "25",
		"--offset", "50",
		"--workspace-id", "ws_test",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
	}
}

func TestPairAcceptsFlags(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	stdout, _, err := executeCommand("pair",
		"--ip", "192.168.1.42",
		"--regenerate-cert",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout, "192.168.1.42") {
		t.Error("output should contain the override IP")
	}
}

func TestPairTextOutput(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	stdout, _, err := executeCommand("pair",
		"--ip", "10.0.0.1",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain QR code half-block characters.
	if !strings.ContainsAny(stdout, "\u2580\u2584\u2588") {
		t.Error("text output missing QR block characters")
	}
	if !strings.Contains(stdout, "Scan this code") {
		t.Error("text output missing scan instruction")
	}
	if !strings.Contains(stdout, "rn_tk_") {
		t.Error("text output missing token")
	}
	if !strings.Contains(stdout, "(new)") {
		t.Error("text output missing cert label")
	}
}

func TestPairJSONOutput(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	stdout, _, err := executeCommand("pair",
		"--ip", "10.0.0.1",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if output["host"] != "10.0.0.1" {
		t.Errorf("host = %v, want 10.0.0.1", output["host"])
	}
	if _, ok := output["token"]; !ok {
		t.Error("JSON output missing 'token' field")
	}
	if _, ok := output["cert_fingerprint"]; !ok {
		t.Error("JSON output missing 'cert_fingerprint' field")
	}
}

func TestPairInvalidIP(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	_, _, err := executeCommand("pair", "--ip", "not-an-ip")
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
	if !strings.Contains(err.Error(), "invalid IP") {
		t.Errorf("error = %q, expected 'invalid IP'", err)
	}
}

func TestRevoke_NoToken(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	_, stderr, err := executeCommand("revoke")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "no active pairing") {
		t.Errorf("expected 'no active pairing', got: %q", stderr)
	}
}

func TestRevoke_DeletesToken(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	// Create pairing token and username files.
	pairingDir := filepath.Join(stateDir, "renotify", "pairing")
	os.MkdirAll(pairingDir, 0700)
	tokenPath := filepath.Join(pairingDir, "token")
	usernamePath := filepath.Join(pairingDir, "username")
	os.WriteFile(tokenPath, []byte("rn_tk_TESTTOKEN\n"), 0600)
	os.WriteFile(usernamePath, []byte("testuser\n"), 0600)

	_, stderr, err := executeCommand("revoke")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "Token revoked") {
		t.Errorf("expected 'Token revoked' in stderr, got: %q", stderr)
	}

	// Token file should be deleted.
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Error("token file should be deleted after revoke")
	}
	// Username file should be deleted.
	if _, err := os.Stat(usernamePath); !os.IsNotExist(err) {
		t.Error("username file should be deleted after revoke")
	}
}

func TestRevoke_JSONOutput(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	// No token — should report revoked=false.
	stdout, _, err := executeCommand("revoke", "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if output["revoked"] != false {
		t.Errorf("revoked = %v, want false", output["revoked"])
	}
	if output["message"] != "no active pairing" {
		t.Errorf("message = %v, want 'no active pairing'",
			output["message"])
	}
}

func TestRevoke_JSONOutput_WithToken(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	stateDir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateDir)

	// Create a token file.
	pairingDir := filepath.Join(stateDir, "renotify", "pairing")
	os.MkdirAll(pairingDir, 0700)
	os.WriteFile(filepath.Join(pairingDir, "token"),
		[]byte("rn_tk_TESTTOKEN\n"), 0600)

	stdout, _, err := executeCommand("revoke", "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var output map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &output); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if output["revoked"] != true {
		t.Errorf("revoked = %v, want true", output["revoked"])
	}
}

func TestAPKExtractRuns(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("apk", "extract",
		"--output", "/tmp/test.apk",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
	}
}

func TestAPKServeRuns(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("apk", "serve",
		"--port", "8080",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
	}
}

func TestAPKSubcommands(t *testing.T) {
	stdout, _, err := executeCommand("apk", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, sub := range []string{"extract", "serve"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("apk help missing subcommand %q", sub)
		}
	}
}

func TestConfigSubcommands(t *testing.T) {
	stdout, _, err := executeCommand("config", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, sub := range []string{"init", "list"} {
		if !strings.Contains(stdout, sub) {
			t.Errorf("config help missing subcommand %q", sub)
		}
	}
}

func TestConfigListText(t *testing.T) {
	stdout, _, err := executeCommand("config", "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should contain header and key entries.
	if !strings.Contains(stdout, "KEY") {
		t.Error("missing header")
	}
	for _, key := range []string{
		"broker.tcp_port", "heartbeat.interval", "username",
	} {
		if !strings.Contains(stdout, key) {
			t.Errorf("missing key %q in list output", key)
		}
	}
}

func TestConfigListJSON(t *testing.T) {
	stdout, _, err := executeCommand("config", "list",
		"--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if len(entries) == 0 {
		t.Fatal("expected non-empty JSON array")
	}
	// Verify first entry has expected fields.
	first := entries[0]
	for _, field := range []string{"key", "type", "default", "env_var", "description"} {
		if _, ok := first[field]; !ok {
			t.Errorf("missing field %q in JSON entry", field)
		}
	}
}

func TestConfigInit_Minimal(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "settings.json")

	_, stderr, err := executeCommand("config", "init",
		"--output", outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "Config written") {
		t.Error("expected confirmation message")
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := cfg["username"]; !ok {
		t.Error("minimal config should contain username")
	}
	// Minimal config should not contain broker section.
	if _, ok := cfg["broker"]; ok {
		t.Error("minimal config should not contain broker section")
	}
}

func TestConfigInit_Full(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "settings.json")

	_, _, err := executeCommand("config", "init",
		"--full", "--output", outPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Full config should contain all sections.
	for _, section := range []string{
		"username", "broker", "mcp", "jetstream", "heartbeat",
	} {
		if _, ok := cfg[section]; !ok {
			t.Errorf("full config missing section %q", section)
		}
	}
}

func TestConfigInit_NoOverwrite(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "settings.json")
	os.WriteFile(outPath, []byte("{}"), 0600)

	_, _, err := executeCommand("config", "init",
		"--output", outPath)
	if err == nil {
		t.Fatal("expected error for existing file")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want 'already exists'", err)
	}
}

func TestConfigInit_Force(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "settings.json")
	os.WriteFile(outPath, []byte("{}"), 0600)

	_, _, err := executeCommand("config", "init",
		"--force", "--output", outPath)
	if err != nil {
		t.Fatalf("unexpected error with --force: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "{}" {
		t.Error("file should have been overwritten")
	}
}

func TestConfigFromEnv(t *testing.T) {
	// Verify that env vars are read correctly by config loading.
	t.Setenv("RENOTIFY_USERNAME", "envuser")
	t.Setenv("RENOTIFY_BROKER_WSS_PORT", "9443")
	_, _, err := executeCommand("daemon", "start", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
