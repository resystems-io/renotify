package command

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	qrterminal "github.com/mdp/qrterminal/v3"

	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/netutil"
	"go.resystems.io/renotify/internal/pairing"
	"go.resystems.io/renotify/internal/xdg"
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
The daemon must be restarted to pick up the new token (automatic
hot-reload is planned for a future release).

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
			notifyDaemon(cmd)
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

// notifyDaemon sends SIGHUP to the running daemon (if any) to
// trigger an auth reload. If the daemon is not running, prints
// a hint to start it.
func notifyDaemon(cmd *cobra.Command) {
	pidPath := xdg.PIDPath()
	data, err := os.ReadFile(pidPath)
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"Note: no running daemon detected. "+
				"Start with: renotify daemon start")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	if err := proc.Signal(syscall.SIGHUP); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Note: could not signal daemon (PID %d): %v\n"+
				"Restart with: renotify daemon stop && "+
				"renotify daemon start\n", pid, err)
		return
	}

	fmt.Fprintf(cmd.ErrOrStderr(),
		"Daemon (PID %d) notified to reload auth.\n", pid)
}
