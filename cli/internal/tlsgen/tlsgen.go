// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package tlsgen generates self-signed TLS certificates for the
// Renotify daemon's WSS listener. See
// docs/analysis-nats-transport-design.md Section 5.
package tlsgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

// GenerateParams configures certificate generation.
type GenerateParams struct {
	DaemonID  string   // used in Subject CN: "renotify-{daemon_id}"
	IPs       []net.IP // Subject Alternative Name IP addresses
	Hostnames []string // Subject Alternative Name DNS names
}

// GenerateResult holds the generated certificate and key material.
type GenerateResult struct {
	CertPEM     []byte // PEM-encoded certificate
	KeyPEM      []byte // PEM-encoded EC private key
	CertDER     []byte // raw DER-encoded certificate
	Fingerprint string // hex-encoded SHA-256 of CertDER
}

// Generate creates an ECDSA P-256 key pair and a self-signed X.509
// certificate with the specified SANs. The certificate is valid for
// 3 years (1,095 days) with a random 128-bit serial number.
func Generate(params GenerateParams) (*GenerateResult, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "renotify-" + params.DaemonID,
		},
		NotBefore:             now,
		NotAfter:              now.Add(1095 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           params.IPs,
		DNSNames:              params.Hostnames,
	}

	certDER, err := x509.CreateCertificate(
		rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return &GenerateResult{
		CertPEM:     certPEM,
		KeyPEM:      keyPEM,
		CertDER:     certDER,
		Fingerprint: Fingerprint(certDER),
	}, nil
}

// Fingerprint computes the SHA-256 hash of DER-encoded certificate
// bytes and returns it as a lowercase hex string (64 characters).
func Fingerprint(certDER []byte) string {
	h := sha256.Sum256(certDER)
	return hex.EncodeToString(h[:])
}

// FingerprintFromPEMFile reads a PEM certificate file and returns
// the SHA-256 fingerprint of the first CERTIFICATE block.
func FingerprintFromPEMFile(certPath string) (string, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return "", fmt.Errorf("read cert: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("no CERTIFICATE block in %s", certPath)
	}

	return Fingerprint(block.Bytes), nil
}
