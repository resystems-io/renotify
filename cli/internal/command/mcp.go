package command

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nats-io/nats.go"
	"github.com/spf13/cobra"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/crockford"

	"github.com/google/uuid"
)

func newMCPCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run a stdio MCP gateway to the daemon",
		Long: `Start a stdio-to-daemon MCP bridge. JSON-RPC messages
are read from stdin and relayed to the daemon's MCP server
via NATS. Responses are written to stdout.

This command is designed to be launched by MCP clients that
use the stdio transport (e.g. Antigravity, Cursor, Claude
Desktop). It runs until stdin is closed or the daemon
connection is lost.

Example client configuration (Antigravity):

  {
    "mcpServers": {
      "renotify": {
        "command": "renotify",
        "args": ["mcp"]
      }
    }
  }

Daemon restart behaviour: exit immediately. The MCP client
detects the subprocess exit and relaunches. See the design
documentation for future session state persistence notes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCP(app)
		},
		// Suppress Cobra usage on runtime errors.
		SilenceUsage: true,
	}
	return cmd
}

// generateSessionID creates a unique stdio MCP session ID.
// Format: ms_ + 26 Crockford Base32 chars (128-bit UUIDv7).
func generateSessionID() string {
	id := uuid.Must(uuid.NewV7())
	return "ms_" + crockford.EncodeBits(id[:], 128)
}

func runMCP(app *App) error {
	cfg := app.Config

	// Connect to NATS (daemon must be running).
	nc, err := broker.ConnectCLI(cfg)
	if err != nil {
		return fmt.Errorf(
			"cannot connect to daemon: %w\n"+
				"Start with: renotify daemon start", err)
	}
	defer nc.Drain()

	sessionID := generateSessionID()
	username := cfg.Username

	c2sSubj := broker.MCPClientToServerSubject(username, sessionID)
	s2cSubj := broker.MCPServerToClientSubject(username, sessionID)
	openSubj := broker.MCPSessionOpenSubject(username)
	closeSubj := broker.MCPSessionCloseSubject(username)

	// Subscribe to server-to-client messages before opening
	// the session to avoid a race.
	s2cSub, err := nc.Subscribe(s2cSubj, func(msg *nats.Msg) {
		// Write each message as one NDJSON line to stdout.
		os.Stdout.Write(msg.Data)
		os.Stdout.Write([]byte("\n"))
	})
	if err != nil {
		return fmt.Errorf("subscribe s2c: %w", err)
	}
	defer s2cSub.Unsubscribe()

	// Publish session open.
	openPayload, _ := json.Marshal(struct {
		SessionID string `json:"session_id"`
	}{SessionID: sessionID})

	if err := nc.Publish(openSubj, openPayload); err != nil {
		return fmt.Errorf("publish open: %w", err)
	}
	nc.Flush()

	// Ensure clean close on exit.
	defer func() {
		closePayload, _ := json.Marshal(struct {
			SessionID string `json:"session_id"`
		}{SessionID: sessionID})
		nc.Publish(closeSubj, closePayload)
		nc.Flush()
	}()

	// Signal handling: exit on SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Stdin relay: read NDJSON lines and publish to c2s.
	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		scanner := bufio.NewScanner(os.Stdin)
		// MCP messages can be large (tool results with file
		// contents). Default 64KB is too small.
		scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}
			if err := nc.Publish(c2sSubj, line); err != nil {
				fmt.Fprintf(os.Stderr,
					"renotify mcp: publish error: %v\n", err)
				return
			}
		}
	}()

	// NATS disconnect detection.
	disconnCh := make(chan struct{}, 1)
	nc.SetDisconnectErrHandler(func(_ *nats.Conn, err error) {
		if err != nil {
			fmt.Fprintf(os.Stderr,
				"renotify mcp: daemon connection lost: %v\n", err)
		}
		select {
		case disconnCh <- struct{}{}:
		default:
		}
	})

	// Block until stdin closes, signal, or NATS disconnect.
	select {
	case <-stdinDone:
		// MCP client closed stdin — normal exit.
	case <-sigCh:
		fmt.Fprintf(os.Stderr, "renotify mcp: interrupted\n")
	case <-disconnCh:
		// Daemon stopped or NATS connection lost.
		// Exit immediately — MCP client will relaunch.
		return fmt.Errorf("daemon connection lost")
	}

	return nil
}
