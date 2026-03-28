package command

import (
	"bytes"
	"strings"
	"testing"
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
	for _, cmd := range []string{"daemon", "post", "ask", "history", "pair", "revoke", "extract-apk"} {
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
		{"daemon", []string{"--foreground", "--username"}},
		{"post", []string{"--title", "--body", "--priority", "--source", "--format"}},
		{"ask", []string{"--title", "--body", "--priority", "--actions", "--response-types", "--timeout", "--format"}},
		{"history", []string{"--workspace-id", "--flow-id", "--since", "--until", "--limit", "--offset", "--format"}},
		{"pair", []string{"--ip", "--regenerate-cert", "--format"}},
		{"revoke", []string{"--format"}},
		{"extract-apk", []string{"--output"}},
	}

	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			stdout, _, err := executeCommand(tc.name, "--help")
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
	_, _, err := executeCommand("daemon", "--foreground")
	if err == nil {
		t.Fatal("expected error for missing username")
	}
	if !strings.Contains(err.Error(), "username") {
		t.Errorf("error should mention username, got: %v", err)
	}
}

func TestDaemonAcceptsUsername(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("daemon", "--foreground")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
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
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("post",
		"--title", "Build done",
		"--body", "All tests passed",
		"--priority", "high",
		"--source", "ci/pipeline",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
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
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("ask",
		"--title", "Deploy?",
		"--response-types", "boolean,text",
		"--timeout", "10m",
		"--priority", "high",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
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
	_, stderr, err := executeCommand("pair",
		"--ip", "192.168.1.42",
		"--regenerate-cert",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
	}
}

func TestRevokeRuns(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("revoke")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
	}
}

func TestExtractAPKRuns(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, stderr, err := executeCommand("extract-apk",
		"--output", "/tmp/test.apk",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "envuser")
	t.Setenv("RENOTIFY_BROKER_WSS_PORT", "9443")
	_, stderr, err := executeCommand("daemon", "--foreground")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr, "not yet implemented") {
		t.Error("expected stub message on stderr")
	}
}
