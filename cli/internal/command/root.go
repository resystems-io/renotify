// Package command defines the Cobra command tree for the Renotify
// CLI. Each subcommand is a separate file; this file creates the
// root command and wires them together.
package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/config"
	"go.resystems.io/renotify/internal/exitcode"
)

// App holds shared state that is initialised by the root command's
// PersistentPreRunE and passed to all subcommands. This avoids
// global state and makes commands testable.
type App struct {
	Config     *config.Config
	ConfigPath string
}

// NewRoot creates the root Cobra command and registers all
// subcommands. Config loading and validation run in
// PersistentPreRunE before any subcommand executes.
func NewRoot() *cobra.Command {
	app := &App{}

	root := &cobra.Command{
		Use:   "renotify",
		Short: "Human-in-the-loop notification system for developer workflows",
		Long: `Renotify sends notifications from CLI scripts and AI agents to
an Android mobile app, captures human decisions, and routes them
back to the originating pipeline.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(app.ConfigPath)
			if err != nil {
				return &exitcode.CodedError{
					Code:    exitcode.Error,
					Message: fmt.Sprintf("config: %v", err),
				}
			}
			app.Config = cfg
			return nil
		},
	}

	root.PersistentFlags().StringVar(
		&app.ConfigPath, "config", "",
		"path to settings.json (default: $XDG_CONFIG_HOME/renotify/settings.json)",
	)

	root.AddCommand(
		newDaemonCmd(app),
		newPostCmd(app),
		newAskCmd(app),
		newAnswerCmd(app),
		newInterjectCmd(app),
		newFlowsCmd(app),
		newHistoryCmd(app),
		newPairCmd(app),
		newRevokeCmd(app),
		newAPKCmd(app),
		newConfigCmd(app),
	)

	return root
}

// Execute runs the root command and exits with the appropriate
// exit code. This is the single entry point called from main.
func Execute() {
	root := NewRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitcode.ExitCode(err))
	}
}
