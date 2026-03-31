package command

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/state"
	"go.resystems.io/renotify/internal/xdg"
)

func newRevokeCmd(app *App) *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "revoke",
		Short: "Revoke the active mobile pairing token",
		Long: `Remove the active pairing token and disconnect the mobile
client. For the embedded broker the token is removed from the NATS
auth configuration immediately via SIGHUP. For a shared broker the
local token file is deleted but the operator must revoke on the
broker side.

This command is idempotent: running it when no pairing exists
prints a message and exits successfully.

Output formats:

  --format text (default):  Status messages to stderr
  --format json:            Machine-readable JSON to stdout`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config

			tokenPath := xdg.PairingTokenPath()
			usernamePath := xdg.PairingUsernamePath()

			// Check for existing token.
			token, err := state.LoadPairingToken(tokenPath)
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"read pairing token: %v", err)
			}

			if token == "" {
				return writeRevokeOutput(cmd, format, false,
					"no active pairing", cfg.Broker.Enabled)
			}

			// Delete token and username files.
			if err := state.DeletePairingToken(tokenPath); err != nil {
				return exitcode.Errorf(exitcode.Error, "%v", err)
			}
			if err := state.DeletePairingUsername(usernamePath); err != nil {
				return exitcode.Errorf(exitcode.Error, "%v", err)
			}

			// Signal daemon to reload auth (removes mobile
			// account and disconnects client).
			if cfg.Broker.Enabled {
				result, pid := signalDaemonReload()
				switch result {
				case signalSent:
					fmt.Fprintf(cmd.ErrOrStderr(),
						"Daemon (PID %d) notified — mobile "+
							"client will be disconnected.\n", pid)
				case signalNoDaemon:
					fmt.Fprintln(cmd.ErrOrStderr(),
						"Note: no running daemon detected. "+
							"Token revoked from disk.")
				case signalFailed:
					fmt.Fprintf(cmd.ErrOrStderr(),
						"Warning: could not signal daemon "+
							"(PID %d). Token revoked from disk "+
							"but daemon still has old auth. "+
							"Restart with: renotify daemon stop "+
							"&& renotify daemon start\n", pid)
				}
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"Shared broker mode: local token deleted. "+
						"The operator must revoke the token on "+
						"the broker side.")
			}

			return writeRevokeOutput(cmd, format, true,
				"pairing token revoked", cfg.Broker.Enabled)
		},
	}

	cmd.Flags().StringVar(&format, "format", "text",
		"output format: json|text")

	return cmd
}

type revokeJSONOutput struct {
	Revoked  bool   `json:"revoked"`
	Message  string `json:"message"`
	Embedded bool   `json:"embedded_broker"`
}

func writeRevokeOutput(cmd *cobra.Command, format string,
	revoked bool, message string, embedded bool) error {

	switch format {
	case "json":
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(revokeJSONOutput{
			Revoked:  revoked,
			Message:  message,
			Embedded: embedded,
		})
	default:
		if revoked {
			fmt.Fprintln(cmd.ErrOrStderr(), "Token revoked.")
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), message)
		}
		return nil
	}
}
