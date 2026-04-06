// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

func newVersionCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the build version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			commit, dirty := vcsInfo()
			if dirty {
				commit += " (dirty)"
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"renotify %s\n", commit)
			return nil
		},
	}
}

// vcsInfo extracts the VCS revision from Go's embedded build
// info. Returns "unknown" if not available (e.g. go run).
func vcsInfo() (commit string, dirty bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown", false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			commit = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if commit == "" {
		return "unknown", false
	}
	// Short hash (7 chars) like git log --oneline.
	if len(commit) > 7 {
		commit = commit[:7]
	}
	return commit, dirty
}
