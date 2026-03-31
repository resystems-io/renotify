package command

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/xdg"
)

// signalDaemonResult describes the outcome of signalling the
// daemon process.
type signalDaemonResult int

const (
	signalSent     signalDaemonResult = iota // SIGHUP delivered
	signalNoDaemon                           // no PID file or stale PID
	signalFailed                             // process exists but signal failed
)

// signalDaemonReload sends SIGHUP to the running daemon to trigger
// an auth reload. Returns the outcome so the caller can print
// context-appropriate messages.
func signalDaemonReload() (signalDaemonResult, int) {
	pidPath := xdg.PIDPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return signalNoDaemon, 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return signalNoDaemon, 0
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return signalNoDaemon, pid
	}

	// Check if process is alive (signal 0).
	if proc.Signal(syscall.Signal(0)) != nil {
		return signalNoDaemon, pid
	}

	if err := proc.Signal(syscall.SIGHUP); err != nil {
		return signalFailed, pid
	}

	return signalSent, pid
}

// notifyDaemonAfterPair signals the daemon and prints
// pair-specific messages.
func notifyDaemonAfterPair(cmd *cobra.Command) {
	result, pid := signalDaemonReload()
	switch result {
	case signalSent:
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Daemon (PID %d) notified to reload auth.\n", pid)
	case signalNoDaemon:
		fmt.Fprintln(cmd.ErrOrStderr(),
			"Note: no running daemon detected. "+
				"Start with: renotify daemon start")
	case signalFailed:
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Note: could not signal daemon (PID %d).\n"+
				"Restart with: renotify daemon stop && "+
				"renotify daemon start\n", pid)
	}
}
