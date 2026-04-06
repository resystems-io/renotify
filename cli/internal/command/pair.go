package command

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/spf13/cobra"

	qrterminal "github.com/mdp/qrterminal/v3"

	"go.resystems.io/renotify/cli/internal/exitcode"
	"go.resystems.io/renotify/cli/internal/netutil"
	"go.resystems.io/renotify/cli/internal/pairing"
	"go.resystems.io/renotify/cli/internal/xdg"
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

If a prior pairing exists, the old token is replaced by a new one.
The running daemon is signalled (SIGHUP) to reload auth, which
disconnects any mobile client using the old token.

Certificate lifecycle:

  First run:            Generates ECDSA P-256 self-signed cert
  Subsequent runs:      Reuses existing cert (same fingerprint)
  --regenerate-cert:    Forces new cert (invalidates prior pairing)

IP discovery:

  By default, the command discovers non-loopback IP addresses on
  active network interfaces and selects a preferred one (IPv4
  private preferred). Use --ip to override:

    renotify pair --ip 192.168.1.42

Output formats:

  --format text (default):  QR code + metadata to stdout
  --format json:            Machine-readable JSON to stdout

Examples:

  renotify pair
  renotify pair --ip 10.0.0.5
  renotify pair --regenerate-cert
  renotify pair --format json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := app.Config

			// Validate --ip if provided.
			if ip != "" {
				if parsed := net.ParseIP(ip); parsed == nil {
					return exitcode.Errorf(exitcode.Error,
						"invalid IP address: %q", ip)
				}
			}

			result, err := pairing.Pair(pairing.Config{
				CertPath:       xdg.TLSCertPath(),
				KeyPath:        xdg.TLSKeyPath(),
				TokenPath:      xdg.PairingTokenPath(),
				UsernamePath:   xdg.PairingUsernamePath(),
				DevicesPath:    xdg.DevicesPath(),
				DaemonIDPath:   xdg.DaemonIDPath(),
				Username:       cfg.Username,
				WSSPort:        cfg.Broker.WSSPort,
				RegenerateCert: regenerateCert,
				OverrideIP:     ip,
				DiscoverIPs:    netutil.DiscoverIPs,
			})
			if err != nil {
				return exitcode.Errorf(exitcode.Error,
					"pair: %v", err)
			}

			out := cmd.OutOrStdout()

			switch format {
			case "json":
				if err := writeJSONOutput(out, result); err != nil {
					return err
				}
			default:
				if err := writeTextOutput(out, result); err != nil {
					return err
				}
			}

			// Signal the running daemon to reload auth config.
			notifyDaemonAfterPair(cmd)
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

func writeTextOutput(w interface{ Write([]byte) (int, error) }, result *pairing.Result) error {
	fmt.Fprintf(w, "Pairing QR code for %s (wss://%s:%d)\n\n",
		result.Username, result.Host, result.Port)

	qrterminal.GenerateHalfBlock(result.PayloadJSON, qrterminal.L, w)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Scan this code with the Renotify app to pair.")

	certLabel := "existing"
	if result.CertRegenerated {
		certLabel = "new"
	}
	fmt.Fprintf(w, "Token: %s (new)\n", result.Token)
	fmt.Fprintf(w, "Cert:  %s (%s)\n", result.CertFingerprint, certLabel)

	return nil
}

type pairJSONOutput struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Token           string `json:"token"`
	CertFingerprint string `json:"cert_fingerprint"`
	CertRegenerated bool   `json:"cert_regenerated"`
	Username        string `json:"username"`
}

func writeJSONOutput(w interface{ Write([]byte) (int, error) }, result *pairing.Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(pairJSONOutput{
		Host:            result.Host,
		Port:            result.Port,
		Token:           result.Token,
		CertFingerprint: result.CertFingerprint,
		CertRegenerated: result.CertRegenerated,
		Username:        result.Username,
	})
}
