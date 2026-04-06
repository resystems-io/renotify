package command

import (
	"os"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/cli/internal/broker"
	"go.resystems.io/renotify/cli/internal/config"
	"go.resystems.io/renotify/cli/internal/exitcode"
	"go.resystems.io/renotify/cli/internal/payload"
	"go.resystems.io/renotify/cli/internal/state"
	"go.resystems.io/renotify/cli/internal/xdg"
)

// flowContext holds the identifiers and NATS connection shared
// by CLI commands that publish to a flow (post, ask).
type flowContext struct {
	cfg            *config.Config
	nc             *nats.Conn
	daemonID       string
	workspaceID    string
	displayName    string
	absPath        string
	flowID         string
	notificationID string
	username       string
}

// setupFlow loads the daemon identity, computes the workspace
// identity from the current working directory, generates flow
// and notification identifiers, and connects to the daemon's
// NATS broker.
func setupFlow(cfg *config.Config) (*flowContext, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, exitcode.Errorf(exitcode.Error,
			"getwd: %v", err)
	}
	return setupFlowFromDir(cfg, cwd)
}

// setupFlowFromDir is like setupFlow but uses the given
// directory instead of os.Getwd(). Used by dispatch to pass
// the hook's cwd.
func setupFlowFromDir(cfg *config.Config, dir string) (*flowContext, error) {
	daemonID, err := state.LoadOrGenerateDaemonID(
		xdg.DaemonIDPath())
	if err != nil {
		return nil, exitcode.Errorf(exitcode.Error,
			"daemon_id: %v", err)
	}

	nc, err := broker.ConnectCLI(cfg)
	if err != nil {
		return nil, exitcode.Errorf(exitcode.Error, "%v", err)
	}

	return &flowContext{
		cfg:            cfg,
		nc:             nc,
		daemonID:       daemonID,
		workspaceID:    state.WorkspaceID(daemonID, dir),
		displayName:    state.DisplayName(dir),
		absPath:        dir,
		flowID:         state.GenerateFlowID(),
		notificationID: state.GenerateNotificationID(),
		username:       cfg.Username,
	}, nil
}

// close drains the NATS connection.
func (fc *flowContext) close() {
	if fc.nc != nil {
		fc.nc.Drain()
	}
}

// workspaceMetadata returns the metadata map carrying workspace
// display name and absolute path for lifecycle events.
func (fc *flowContext) workspaceMetadata() map[string]string {
	return map[string]string{
		payload.MetaDisplayName: fc.displayName,
		payload.MetaAbsPath:     fc.absPath,
	}
}
