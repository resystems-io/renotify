package state

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"

	"go.resystems.io/renotify/cli/internal/crockford"
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

// GenerateToken always generates a new auth token and writes it
// atomically, even if a file already exists. Used by `renotify
// pair` where each pairing revokes the old token by overwriting.
func GenerateToken(path string) (string, error) {
	b := make([]byte, 32) // 256 bits
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	tok := "rn_tk_" + crockford.EncodeBits(b, 256)
	if err := writeAtomic(path, tok); err != nil {
		return "", err
	}
	return tok, nil
}

// WriteUsername writes the pairing username atomically.
func WriteUsername(path, username string) error {
	return writeAtomic(path, username)
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

// DeletePairingToken removes the pairing token file. Returns nil
// if the file does not exist.
func DeletePairingToken(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete pairing token: %w", err)
	}
	return nil
}

// DeletePairingUsername removes the pairing username file. Returns
// nil if the file does not exist.
func DeletePairingUsername(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete pairing username: %w", err)
	}
	return nil
}
