package command

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/mdp/qrterminal/v3"
	"github.com/spf13/cobra"

	apkembed "go.resystems.io/renotify/internal/embed"
	"go.resystems.io/renotify/internal/netutil"
)

func newAPKCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apk",
		Short: "Manage the embedded Android APK",
		Long: `Commands for extracting and serving the Android APK that is
bundled into the CLI binary at build time via go:embed (P-02).`,
	}

	cmd.AddCommand(
		newAPKExtractCmd(app),
		newAPKServeCmd(app),
	)

	return cmd
}

// loadAPK reads the embedded APK or returns an error if the
// binary was built without it (e.g. via go build without make).
func loadAPK() ([]byte, error) {
	data, err := apkembed.FS.ReadFile(apkembed.APKName)
	if err != nil {
		return nil, fmt.Errorf(
			"no APK embedded in this binary — build with " +
				"`make` to include the Android APK")
	}
	return data, nil
}

func newAPKExtractCmd(app *App) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract the embedded Android APK to disk",
		Long: `Write the embedded Android APK file to the specified path so
the user can install it on their device. The APK is bundled into
the CLI binary at build time via go:embed (P-02).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := loadAPK()
			if err != nil {
				return err
			}
			if err := os.WriteFile(output, data, 0644); err != nil {
				return fmt.Errorf("write %s: %w", output, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"APK written to %s (%d bytes)\n",
				output, len(data))
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
			data, err := loadAPK()
			if err != nil {
				return err
			}

			// Determine bind address.
			bindAddr := addr
			if bindAddr == "" {
				ips, _ := netutil.DiscoverIPs()
				bindAddr = netutil.PreferredIP(ips).String()
			}

			// Listen on the requested port (0 = random).
			ln, err := net.Listen("tcp",
				fmt.Sprintf("%s:%d", bindAddr, port))
			if err != nil {
				return fmt.Errorf("listen: %w", err)
			}
			defer ln.Close()

			listenAddr := ln.Addr().String()
			downloadURL := fmt.Sprintf(
				"http://%s/renotify.apk", listenAddr)

			// Serve the APK using http.ServeContent which handles
			// Content-Length, Range requests (required by Firefox),
			// and proper streaming for large files.
			apkModTime := time.Now()
			mux := http.NewServeMux()
			mux.HandleFunc("/renotify.apk",
				func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type",
						"application/vnd.android.package-archive")
					w.Header().Set("Content-Disposition",
						"attachment; filename=renotify.apk")
					http.ServeContent(w, r, "renotify.apk",
						apkModTime, bytes.NewReader(data))
				})

			srv := &http.Server{Handler: mux}
			go srv.Serve(ln)

			// Extract the actual port from the listener
			// (may differ from the flag when port=0).
			_, listenPort, _ := net.SplitHostPort(listenAddr)

			// Print URL and QR code.
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Serving APK at: %s\n", downloadURL)
			fmt.Fprintf(out, "Scan the QR code to download:\n\n")
			qrterminal.GenerateHalfBlock(downloadURL, qrterminal.L, out)

			if runtime.GOOS == "linux" {
				fmt.Fprintf(out, "\nFirewall: allow temporary access then remove after download:\n")
				fmt.Fprintf(out, "  sudo ufw allow %s/tcp comment 'renotify apk serve'\n", listenPort)
				fmt.Fprintf(out, "  sudo ufw delete allow %s/tcp\n", listenPort)
			}

			fmt.Fprintf(out, "\nPress Ctrl-C to stop.\n")

			// Block until Ctrl-C.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh

			fmt.Fprintf(out, "\nShutting down.\n")
			return srv.Close()
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "",
		"bind address (default: auto-discovered LAN IP)")
	cmd.Flags().IntVar(&port, "port", 0,
		"listen port (default: random available port)")

	return cmd
}
