// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

package command

import "github.com/spf13/cobra"

// newAppCmd creates the "renotify app" command group that
// holds platform-specific client commands. Currently contains
// "apk" for Android; extensible for "ios" in the future.
func newAppCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Manage embedded client applications",
		Long: `Commands for extracting and serving the mobile client
applications that are bundled into the CLI binary at build
time via go:embed.

Currently supports the Android APK. Use "renotify app apk"
for Android-specific commands.`,
	}

	cmd.AddCommand(newAPKCmd(app))

	return cmd
}
