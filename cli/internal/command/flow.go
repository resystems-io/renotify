package command

import (
	"os"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/broker"
	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/state"
	"go.resystems.io/renotify/internal/xdg"
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
	daemonID, err := state.LoadOrGenerateDaemonID(
		xdg.DaemonIDPath())
	if err != nil {
		return nil, exitcode.Errorf(exitcode.Error,
			"daemon_id: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, exitcode.Errorf(exitcode.Error,
			"getwd: %v", err)
	}

	nc, err := broker.ConnectCLI(cfg)
	if err != nil {
		return nil, exitcode.Errorf(exitcode.Error, "%v", err)
	}

	return &flowContext{
		cfg:            cfg,
		nc:             nc,
		daemonID:       daemonID,
		workspaceID:    state.WorkspaceID(daemonID, cwd),
		displayName:    state.DisplayName(cwd),
		absPath:        cwd,
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

// Well-known metadata keys for workspace context in lifecycle
// events. The daemon's registry extracts these when registering
// a flow (C-10).
const (
	MetaDisplayName = "workspace_display_name"
	MetaAbsPath     = "workspace_abs_path"
)

// workspaceMetadata returns the metadata map carrying workspace
// display name and absolute path for lifecycle events.
func (fc *flowContext) workspaceMetadata() map[string]string {
	return map[string]string{
		MetaDisplayName: fc.displayName,
		MetaAbsPath:     fc.absPath,
	}
}
