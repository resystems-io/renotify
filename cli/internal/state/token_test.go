package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrGenerateToken_GeneratesOnMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	tok, err := LoadOrGenerateToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok, "rn_tk_") {
		t.Errorf("token should start with rn_tk_, got %q", tok)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file perm = %o, want 0600", perm)
	}
}

func TestLoadOrGenerateToken_LoadsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	os.WriteFile(path, []byte("rn_tk_EXISTING\n"), 0600)

	tok, err := LoadOrGenerateToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "rn_tk_EXISTING" {
		t.Errorf("got %q, want %q", tok, "rn_tk_EXISTING")
	}
}

func TestLoadOrGenerateToken_Format(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	tok, err := LoadOrGenerateToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) != 58 {
		t.Errorf("token length = %d, want 58", len(tok))
	}
	body := strings.TrimPrefix(tok, "rn_tk_")
	if len(body) != 52 {
		t.Errorf("body length = %d, want 52", len(body))
	}
	for i, c := range body {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", c) {
			t.Errorf("char %d %q not in Crockford alphabet", i, string(c))
		}
	}
}

func TestLoadPairingToken_ReturnsEmptyOnMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent")
	tok, err := LoadPairingToken(path)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if tok != "" {
		t.Errorf("expected empty string, got %q", tok)
	}
}

func TestLoadPairingToken_LoadsExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	os.WriteFile(path, []byte("rn_tk_PAIRING\n"), 0600)

	tok, err := LoadPairingToken(path)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "rn_tk_PAIRING" {
		t.Errorf("got %q, want %q", tok, "rn_tk_PAIRING")
	}
}
