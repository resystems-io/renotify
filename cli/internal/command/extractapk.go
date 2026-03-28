package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newExtractAPKCmd(app *App) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "extract-apk",
		Short: "Extract the embedded Android APK to disk",
		Long: `Write the embedded Android APK file to the specified path so
the user can install it on their device. The APK is bundled into
the CLI binary at build time via go:embed (R-PKG-02).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = app.Config
			_ = output

			fmt.Fprintln(cmd.ErrOrStderr(), "extract-apk: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "renotify.apk",
		"output file path for the APK")

	return cmd
}
