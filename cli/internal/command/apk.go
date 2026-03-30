package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAPKCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apk",
		Short: "Manage the embedded Android APK",
		Long: `Commands for extracting and serving the Android APK that is
bundled into the CLI binary at build time via go:embed (R-PKG-02).`,
	}

	cmd.AddCommand(
		newAPKExtractCmd(app),
		newAPKServeCmd(app),
	)

	return cmd
}

func newAPKExtractCmd(app *App) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract the embedded Android APK to disk",
		Long: `Write the embedded Android APK file to the specified path so
the user can install it on their device. The APK is bundled into
the CLI binary at build time via go:embed (R-PKG-02).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = app.Config
			_ = output

			fmt.Fprintln(cmd.ErrOrStderr(), "apk extract: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "renotify.apk",
		"output file path for the APK")

	return cmd
}

func newAPKServeCmd(app *App) *cobra.Command {
	var (
		addr string
		port int
	)

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the embedded APK over HTTP with a QR code",
		Long: `Start a temporary HTTP server that serves the embedded Android
APK and display a QR code containing the download URL. The user
can scan the QR code with their phone browser to download and
install the APK directly.

The server runs until interrupted (Ctrl-C).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = app.Config
			_ = addr
			_ = port

			fmt.Fprintln(cmd.ErrOrStderr(), "apk serve: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "",
		"bind address (default: auto-discovered LAN IP)")
	cmd.Flags().IntVar(&port, "port", 0,
		"listen port (default: random available port)")

	return cmd
}
