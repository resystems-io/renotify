package command

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/daemon"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/httpserver"
	"go.resystems.io/renotify/internal/mcpserver"
	"go.resystems.io/renotify/internal/xdg"
)

func newDaemonCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the Renotify daemon",
		Long: `Manage the daemon process that runs the embedded NATS broker,
MCP server, and background services.

Subcommands: start, stop, status.

Diagnostics (NATS):

  The embedded broker authenticates with username "daemon" and the
  internal token stored at $XDG_STATE_HOME/renotify/internal_token.
  Subjects are scoped to resystems.renotify.<username>.>

  Read the token:

    TOKEN=$(cat ~/.local/state/renotify/internal_token)

  Publish and subscribe (replace <username> with your config username):

    nats sub "resystems.renotify.<username>.test" \
      --user daemon --password "$TOKEN"
    nats pub "resystems.renotify.<username>.test" "hello" \
      --user daemon --password "$TOKEN"

  List JetStream streams and consumers:

    nats stream ls --user daemon --password "$TOKEN"
    nats consumer ls RENOTIFY --user daemon --password "$TOKEN"

  Connection and account info:

    nats account info --user daemon --password "$TOKEN"

Diagnostics (WSS — mobile connection path):

  The WSS listener (0.0.0.0:4223) is used by the Android app.
  Test the mobile connection path from the CLI using the pairing
  token and the self-signed certificate:

  Read the pairing token:

    PAIRING_TOKEN=$(cat ~/.local/state/renotify/pairing/token)

  Subscribe via WSS (loopback):

    nats sub "resystems.renotify.<username>.>" \
      --server "wss://127.0.0.1:4223" \
      --user mobile --password "$PAIRING_TOKEN" \
      --tlsca ~/.local/state/renotify/tls/cert.pem

  Subscribe via WSS (LAN IP, simulates phone connection):

    nats sub "resystems.renotify.<username>.>" \
      --server "wss://<lan-ip>:4223" \
      --user mobile --password "$PAIRING_TOKEN" \
      --tlsca ~/.local/state/renotify/tls/cert.pem

  If the WSS connection succeeds from the CLI but the Android app
  fails, the issue is between the phone and the host (firewall,
  routing, or TLS fingerprint mismatch after cert regeneration).

Diagnostics (MCP):

  The MCP server listens on http://127.0.0.1:4224/mcp via the
  Streamable HTTP transport. Test with curl:

    curl -s -X POST http://127.0.0.1:4224/mcp \
      -H "Content-Type: application/json" \
      -H "Accept: application/json, text/event-stream" \
      -d '{"jsonrpc":"2.0","id":1,"method":"initialize",
           "params":{"protocolVersion":"2025-06-18",
           "capabilities":{},
           "clientInfo":{"name":"curl-test","version":"1.0"}}}'`,
	}

	cmd.AddCommand(
		newDaemonStartCmd(app),
		newDaemonStopCmd(),
		newDaemonStatusCmd(),
	)

	return cmd
}

func newDaemonStartCmd(app *App) *cobra.Command {
	var (
		foreground bool
		username   string
		logLevel   string
		sharedURL  string
		disableMCP bool
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon",
		Long: `Start the daemon process that runs the embedded NATS broker,
MCP server, and background services. Runs in the background by
default; use --foreground to run in the foreground with logs to
stderr.

Broker mode:

  By default the daemon starts an embedded NATS broker on
  127.0.0.1:4222 (TCP) and 0.0.0.0:4223 (WSS). To connect to an
  external shared broker instead, pass --shared-broker:

    renotify daemon start --shared-broker nats://broker:4222

  This disables the embedded broker and uses the shared broker URL.
  Credentials can be set via environment variables or settings.json:

    RENOTIFY_SHARED_BROKER_USERNAME / RENOTIFY_SHARED_BROKER_PASSWORD
    RENOTIFY_SHARED_BROKER_TOKEN

MCP server:

  The daemon runs an MCP server via SSE on 127.0.0.1:4224/mcp by
  default. AI agents (Claude Code, etc.) connect to this endpoint.
  Disable with --no-mcp.

  The MCP host and port are configurable via environment or config:

    RENOTIFY_MCP_HOST / RENOTIFY_MCP_PORT

Logging:

  Log level is configurable via --log-level (debug, info, warn,
  error). Default: info. In foreground mode logs go to stderr as
  human-readable text; in background mode to the log file as JSON.

    RENOTIFY_DAEMON_LOG_LEVEL=debug
    RENOTIFY_DAEMON_LOG_FILE=/path/to/daemon.log

Configuration precedence (highest to lowest):

  1. CLI flags (--username, --shared-broker, --no-mcp, etc.)
  2. Environment variables (RENOTIFY_ prefix, e.g. RENOTIFY_USERNAME)
  3. Config file ($XDG_CONFIG_HOME/renotify/settings.json)
  4. Compiled defaults`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config

			// Apply flag overrides.
			if cmd.Flags().Changed("foreground") {
				cfg.Daemon.Foreground = foreground
			}
			if cmd.Flags().Changed("username") {
				cfg.Username = username
			}
			if cmd.Flags().Changed("log-level") {
				cfg.Daemon.LogLevel = logLevel
			}
			if cmd.Flags().Changed("shared-broker") {
				cfg.Broker.Enabled = false
				cfg.SharedBroker.URL = sharedURL
			}
			if cmd.Flags().Changed("no-mcp") {
				cfg.MCP.Enabled = !disableMCP
			}

			if err := cfg.Validate(); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"config: %v", err)
			}

			// Check if already running.
			if pid, ok := readPID(); ok {
				if processRunning(pid) {
					return exitcode.Errorf(exitcode.Error,
						"daemon already running (pid %d)", pid)
				}
				// Stale PID file — clean up.
				os.Remove(xdg.PIDPath())
			}

			// Background mode: re-exec as a detached child.
			// The child detects RENOTIFY_DAEMON_CHILD=1 and
			// skips re-daemonization, running with log-to-file.
			if !cfg.Daemon.Foreground &&
				os.Getenv("RENOTIFY_DAEMON_CHILD") != "1" {
				return daemonize(cmd)
			}

			return runDaemon(cmd, cfg)
		},
	}

	cmd.Flags().BoolVar(&foreground, "foreground", false,
		"run in foreground (logs to stderr)")
	cmd.Flags().StringVar(&username, "username", "",
		"NATS auth identity (default: $USER)")
	cmd.Flags().StringVar(&logLevel, "log-level", "",
		"log level: debug, info, warn, error (default: info)")
	cmd.Flags().StringVar(&sharedURL, "shared-broker", "",
		"use external NATS broker URL (disables embedded broker)")
	cmd.Flags().BoolVar(&disableMCP, "no-mcp", false,
		"disable the MCP server")

	return cmd
}

func newDaemonStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, ok := readPID()
			if !ok {
				return exitcode.Errorf(exitcode.Error,
					"no PID file found (daemon not running?)")
			}
			if !processRunning(pid) {
				os.Remove(xdg.PIDPath())
				return exitcode.Errorf(exitcode.Error,
					"daemon not running (stale PID %d removed)", pid)
			}

			proc, err := os.FindProcess(pid)
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"find process %d: %v", pid, err)
			}
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				return exitcode.Errorf(exitcode.Error,
					"signal process %d: %v", pid, err)
			}

			// Wait briefly for shutdown.
			for range 20 {
				time.Sleep(250 * time.Millisecond)
				if !processRunning(pid) {
					os.Remove(xdg.PIDPath())
					fmt.Fprintln(cmd.ErrOrStderr(),
						"daemon stopped")
					return nil
				}
			}

			fmt.Fprintf(cmd.ErrOrStderr(),
				"SIGTERM sent to pid %d; waiting for exit\n", pid)
			return nil
		},
	}
}

func newDaemonStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, ok := readPID()
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "stopped")
				return nil
			}
			if !processRunning(pid) {
				os.Remove(xdg.PIDPath())
				fmt.Fprintln(cmd.OutOrStdout(), "stopped (stale PID removed)")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "running (pid %d)\n", pid)
			return nil
		},
	}
}

// runDaemon runs the daemon in the current process (foreground
// mode or as the re-exec'd child).
func runDaemon(cmd *cobra.Command, cfg *config.Config) error {
	logger, cleanup, err := setupLogger(
		cfg.Daemon.Foreground, cfg.Daemon.LogLevel, cfg.Daemon.LogFile)
	if err != nil {
		return exitcode.Errorf(exitcode.Error, "logger: %v", err)
	}
	defer cleanup()

	// Write PID file.
	if err := writePID(); err != nil {
		logger.Warn("failed to write PID file", "err", err)
	}
	defer os.Remove(xdg.PIDPath())

	// Build subsystem list.
	var opts []daemon.Option
	opts = append(opts, daemon.WithLogger(logger))

	if cfg.MCP.Enabled {
		httpSrv := httpserver.New(cfg.MCP.Host, cfg.MCP.Port, logger)
		mcpSrv := mcpserver.New(httpSrv, logger)
		opts = append(opts,
			daemon.WithSubsystem(httpSrv),
			daemon.WithSubsystem(mcpSrv),
		)
	}

	// Signal handling.
	ctx, stop := signal.NotifyContext(cmd.Context(),
		syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctrl := daemon.NewController(cfg, opts...)
	if err := ctrl.Run(ctx); err != nil {
		return exitcode.Errorf(exitcode.Error, "%v", err)
	}
	return nil
}

// daemonize re-executes the current binary as a background process.
func daemonize(cmd *cobra.Command) error {
	exe, err := os.Executable()
	if err != nil {
		return exitcode.Errorf(exitcode.Error,
			"resolve executable: %v", err)
	}

	// Build args: pass through user-set flags but NOT --foreground.
	// The child runs without --foreground so it logs to file.
	// RENOTIFY_DAEMON_CHILD=1 prevents the child from
	// re-daemonizing.
	args := []string{exe, "daemon", "start"}
	cmd.Flags().Visit(func(f *pflag.Flag) {
		if f.Name == "foreground" {
			return // child must not run in foreground mode
		}
		args = append(args, "--"+f.Name, f.Value.String())
	})
	if p := cmd.InheritedFlags().Lookup("config"); p != nil && p.Changed {
		args = append(args, "--config", p.Value.String())
	}

	env := append(os.Environ(), "RENOTIFY_DAEMON_CHILD=1")
	child := &exec.Cmd{
		Path: exe,
		Args: args,
		Env:  env,
		SysProcAttr: &syscall.SysProcAttr{
			Setsid: true,
		},
	}

	// Redirect child stdout/stderr to /dev/null — it logs to
	// file via --foreground + log_file config.
	child.Stdout = nil
	child.Stderr = nil

	if err := child.Start(); err != nil {
		return exitcode.Errorf(exitcode.Error,
			"start background process: %v", err)
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "daemon started (pid %d)\n",
		child.Process.Pid)
	return nil
}

// PID file helpers.

func readPID() (int, bool) {
	data, err := os.ReadFile(xdg.PIDPath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

func writePID() error {
	dir := filepath.Dir(xdg.PIDPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(xdg.PIDPath(),
		[]byte(strconv.Itoa(os.Getpid())+"\n"), 0600)
}

func processRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 tests if the process exists.
	return proc.Signal(syscall.Signal(0)) == nil
}

// setupLogger creates an slog.Logger that writes to stderr
// (foreground) or a log file (background).
func setupLogger(foreground bool, level, logFile string) (*slog.Logger, func(), error) {
	lvl := parseLogLevel(level)
	opts := &slog.HandlerOptions{Level: lvl}

	if foreground {
		handler := slog.NewTextHandler(os.Stderr, opts)
		return slog.New(handler), func() {}, nil
	}

	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file %s: %w", logFile, err)
	}
	handler := slog.NewJSONHandler(f, opts)
	return slog.New(handler), func() { f.Close() }, nil
}

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
