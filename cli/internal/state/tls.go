package state

import (
	"fmt"
	"os"
)

// LoadTLS checks that both cert and key files exist and returns
// their paths. Returns ("", "", nil) if either file is missing
// (caller should log a warning and skip WSS). Returns an error
// only if the file exists but cannot be accessed.
func LoadTLS(certPath, keyPath string) (cert, key string, err error) {
	certOK, err := fileExists(certPath)
	if err != nil {
		return "", "", fmt.Errorf("check TLS cert: %w", err)
	}
	keyOK, err := fileExists(keyPath)
	if err != nil {
		return "", "", fmt.Errorf("check TLS key: %w", err)
	}
	if !certOK || !keyOK {
		return "", "", nil
	}
	return certPath, keyPath, nil
}

// WriteTLS writes the TLS certificate and key files atomically.
// The certificate is written with mode 0644 (readable for NATS
// server), the key with mode 0600 (secret). Parent directories
// are created with mode 0700.
func WriteTLS(certPath string, certPEM []byte, keyPath string, keyPEM []byte) error {
	if err := writeAtomicBytes(certPath, certPEM, 0644); err != nil {
		return fmt.Errorf("write TLS cert: %w", err)
	}
	if err := writeAtomicBytes(keyPath, keyPEM, 0600); err != nil {
		// Best-effort cleanup of cert if key write fails.
		os.Remove(certPath)
		return fmt.Errorf("write TLS key: %w", err)
	}
	return nil
}

func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
