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

func TestWriteTLS_CreatesFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	err := WriteTLS(certPath, []byte("CERT DATA"), keyPath, []byte("KEY DATA"))
	if err != nil {
		t.Fatal(err)
	}

	certData, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(certData) != "CERT DATA" {
		t.Errorf("cert = %q, want %q", certData, "CERT DATA")
	}

	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(keyData) != "KEY DATA" {
		t.Errorf("key = %q, want %q", keyData, "KEY DATA")
	}
}

func TestWriteTLS_CertPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	WriteTLS(certPath, []byte("cert"), keyPath, []byte("key"))

	info, err := os.Stat(certPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0644 {
		t.Errorf("cert perm = %o, want 0644", perm)
	}
}

func TestWriteTLS_KeyPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	WriteTLS(certPath, []byte("cert"), keyPath, []byte("key"))

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("key perm = %o, want 0600", perm)
	}
}

func TestWriteTLS_CreatesDirectories(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "tls")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	err := WriteTLS(certPath, []byte("cert"), keyPath, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestWriteTLS_AtomicOverwrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tls")
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	WriteTLS(certPath, []byte("old cert"), keyPath, []byte("old key"))
	WriteTLS(certPath, []byte("new cert"), keyPath, []byte("new key"))

	certData, _ := os.ReadFile(certPath)
	if string(certData) != "new cert" {
		t.Errorf("cert = %q, want %q", certData, "new cert")
	}
	keyData, _ := os.ReadFile(keyPath)
	if string(keyData) != "new key" {
		t.Errorf("key = %q, want %q", keyData, "new key")
	}
}
