// Package pairing orchestrates the `renotify pair` flow:
// certificate generation, IP discovery, token creation, and
// provisioning payload assembly. See
// docs/analysis-nats-transport-design.md Section 8.3.
package pairing

import (
	"encoding/json"
	"fmt"
	"net"

	"go.resystems.io/renotify/internal/netutil"
	"go.resystems.io/renotify/internal/state"
	"go.resystems.io/renotify/internal/tlsgen"
)

// Config holds all inputs for the pairing flow.
type Config struct {
	CertPath     string // path to TLS cert PEM file
	KeyPath      string // path to TLS key PEM file
	TokenPath    string // path to pairing token file
	UsernamePath string // path to pairing username file
	DaemonIDPath string // path to daemon_id file

	Username string // NATS auth identity
	WSSPort  int    // WSS listener port for payload

	RegenerateCert bool   // force new TLS certificate
	OverrideIP     string // override auto-discovered IP

	// DiscoverIPs is injectable for testing. In production,
	// pass netutil.DiscoverIPs.
	DiscoverIPs func() ([]net.IP, error)
}

// Result holds all outputs of the pairing flow.
type Result struct {
	Host            string
	Port            int
	Token           string
	CertFingerprint string
	CertRegenerated bool
	PayloadJSON     string
	Username        string
}

// ProvisioningPayload is the minified JSON encoded into the QR
// code. Single-character keys minimise QR density (R-API-08).
type ProvisioningPayload struct {
	Version int    `json:"v"`
	Host    string `json:"h"`
	Port    int    `json:"p"`
	Token   string `json:"t"`
	CertSHA string `json:"c"`
}

// Pair executes the full pairing flow: load/generate daemon_id,
// check/generate TLS cert, discover IPs, generate token, store
// username, compute fingerprint, and assemble the provisioning
// payload.
func Pair(cfg Config) (*Result, error) {
	// 1. Load daemon_id (needed for cert CN).
	daemonID, err := state.LoadOrGenerateDaemonID(cfg.DaemonIDPath)
	if err != nil {
		return nil, fmt.Errorf("daemon_id: %w", err)
	}

	// 2. Check existing TLS cert.
	existingCert, _, err := state.LoadTLS(cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("check TLS: %w", err)
	}
	certExists := existingCert != ""
	certRegenerated := false

	// 3. Discover IPs once if needed (for cert SANs or host
	//    selection).
	var discoveredIPs []net.IP
	needDiscovery := !certExists || cfg.RegenerateCert || cfg.OverrideIP == ""
	if needDiscovery {
		discover := cfg.DiscoverIPs
		if discover == nil {
			discover = netutil.DiscoverIPs
		}
		discoveredIPs, err = discover()
		if err != nil {
			return nil, fmt.Errorf("discover IPs: %w", err)
		}
	}

	// 4. Generate cert if needed.
	if !certExists || cfg.RegenerateCert {
		sanIPs := deduplicateIPs(append(discoveredIPs,
			net.ParseIP("127.0.0.1")))

		result, err := tlsgen.Generate(tlsgen.GenerateParams{
			DaemonID:  daemonID,
			IPs:       sanIPs,
			Hostnames: []string{"localhost"},
		})
		if err != nil {
			return nil, fmt.Errorf("generate cert: %w", err)
		}
		if err := state.WriteTLS(cfg.CertPath, result.CertPEM,
			cfg.KeyPath, result.KeyPEM); err != nil {
			return nil, fmt.Errorf("write TLS: %w", err)
		}
		certRegenerated = true
	}

	// 5. Select host for provisioning payload.
	var host string
	if cfg.OverrideIP != "" {
		host = cfg.OverrideIP
	} else {
		host = netutil.PreferredIP(discoveredIPs).String()
	}

	// 6. Always generate a new pairing token (revokes old).
	token, err := state.GenerateToken(cfg.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("generate token: %w", err)
	}

	// 7. Store pairing username.
	if err := state.WriteUsername(cfg.UsernamePath, cfg.Username); err != nil {
		return nil, fmt.Errorf("write username: %w", err)
	}

	// 8. Compute cert fingerprint.
	fingerprint, err := tlsgen.FingerprintFromPEMFile(cfg.CertPath)
	if err != nil {
		return nil, fmt.Errorf("fingerprint: %w", err)
	}

	// 9. Assemble provisioning payload.
	payload := ProvisioningPayload{
		Version: 1,
		Host:    host,
		Port:    cfg.WSSPort,
		Token:   token,
		CertSHA: fingerprint,
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	return &Result{
		Host:            host,
		Port:            cfg.WSSPort,
		Token:           token,
		CertFingerprint: fingerprint,
		CertRegenerated: certRegenerated,
		PayloadJSON:     string(payloadJSON),
		Username:        cfg.Username,
	}, nil
}

// deduplicateIPs removes duplicate IPs from the slice.
func deduplicateIPs(ips []net.IP) []net.IP {
	seen := make(map[string]bool, len(ips))
	out := make([]net.IP, 0, len(ips))
	for _, ip := range ips {
		key := ip.String()
		if !seen[key] {
			seen[key] = true
			out = append(out, ip)
		}
	}
	return out
}
