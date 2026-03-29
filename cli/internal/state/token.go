package state

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"

	"go.resystems.io/renotify/internal/crockford"
)

// LoadOrGenerateToken reads an auth token from path. If the file
// does not exist, it generates a new token (rn_tk_ + 52 Crockford
// Base32 chars from 256 random bits), writes it atomically, and
// returns it.
func LoadOrGenerateToken(path string) (string, error) {
	return loadOrGenerate(path, func() (string, error) {
		b := make([]byte, 32) // 256 bits
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("generate token: %w", err)
		}
		return "rn_tk_" + crockford.EncodeBits(b, 256), nil
	})
}

// LoadPairingToken reads the pairing token from path. Returns
// ("", nil) if the file does not exist.
func LoadPairingToken(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read pairing token: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
