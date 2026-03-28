package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newPairCmd(app *App) *cobra.Command {
	var (
		ip             string
		regenerateCert bool
		format         string
	)

	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Generate a pairing QR code for the mobile app",
		Long: `Create or refresh the mobile pairing. Generates a TLS certificate
(if none exists), creates an auth token, and renders an ASCII QR
code to the terminal containing the provisioning payload. The
mobile app scans this code to establish a secure connection.

If a prior pairing exists, the old token is revoked before a new
one is issued (R-SEC-02).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = app.Config
			_ = ip
			_ = regenerateCert
			_ = format

			fmt.Fprintln(cmd.ErrOrStderr(), "pair: not yet implemented")
			return nil
		},
	}

	cmd.Flags().StringVar(&ip, "ip", "",
		"override discovered local IP address")
	cmd.Flags().BoolVar(&regenerateCert, "regenerate-cert", false,
		"force new TLS certificate (invalidates prior pairing)")
	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}
