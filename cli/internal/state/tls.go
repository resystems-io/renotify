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
