package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTLS_BothExist(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	os.WriteFile(certPath, []byte("cert"), 0644)
	os.WriteFile(keyPath, []byte("key"), 0600)

	cert, key, err := LoadTLS(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if cert != certPath || key != keyPath {
		t.Errorf("got (%q, %q), want (%q, %q)", cert, key, certPath, keyPath)
	}
}

func TestLoadTLS_CertMissing(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.pem")
	os.WriteFile(keyPath, []byte("key"), 0600)

	cert, key, err := LoadTLS(filepath.Join(dir, "cert.pem"), keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if cert != "" || key != "" {
		t.Errorf("expected empty strings, got (%q, %q)", cert, key)
	}
}

func TestLoadTLS_KeyMissing(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	os.WriteFile(certPath, []byte("cert"), 0644)

	cert, key, err := LoadTLS(certPath, filepath.Join(dir, "key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if cert != "" || key != "" {
		t.Errorf("expected empty strings, got (%q, %q)", cert, key)
	}
}

func TestLoadTLS_BothMissing(t *testing.T) {
	dir := t.TempDir()
	cert, key, err := LoadTLS(
		filepath.Join(dir, "cert.pem"),
		filepath.Join(dir, "key.pem"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if cert != "" || key != "" {
		t.Errorf("expected empty strings, got (%q, %q)", cert, key)
	}
}
