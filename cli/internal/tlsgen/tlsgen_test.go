package tlsgen

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testParams() GenerateParams {
	return GenerateParams{
		DaemonID:  "dn_TESTDAEMON01",
		IPs:       []net.IP{net.ParseIP("192.168.1.42"), net.ParseIP("127.0.0.1")},
		Hostnames: []string{"localhost"},
	}
}

func mustGenerate(t *testing.T) *GenerateResult {
	t.Helper()
	r, err := Generate(testParams())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return r
}

func parseCert(t *testing.T, r *GenerateResult) *x509.Certificate {
	t.Helper()
	cert, err := x509.ParseCertificate(r.CertDER)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert
}

func TestGenerate_KeyAlgorithm(t *testing.T) {
	r := mustGenerate(t)
	block, _ := pem.Decode(r.KeyPEM)
	if block == nil {
		t.Fatal("no PEM block in key")
	}
	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("ParseECPrivateKey: %v", err)
	}
	if key.Curve != elliptic.P256() {
		t.Errorf("curve = %v, want P-256", key.Curve.Params().Name)
	}
}

func TestGenerate_CertValidity(t *testing.T) {
	before := time.Now().Add(-time.Minute)
	r := mustGenerate(t)
	after := time.Now().Add(time.Minute)

	cert := parseCert(t, r)

	if cert.NotBefore.Before(before) {
		t.Errorf("NotBefore %v is before test start %v",
			cert.NotBefore, before)
	}
	if cert.NotBefore.After(after) {
		t.Errorf("NotBefore %v is after test end %v",
			cert.NotBefore, after)
	}

	// 3 years = 1095 days.
	expectedAfter := cert.NotBefore.Add(1095 * 24 * time.Hour)
	if !cert.NotAfter.Equal(expectedAfter) {
		t.Errorf("NotAfter = %v, want %v", cert.NotAfter, expectedAfter)
	}
}

func TestGenerate_SubjectCN(t *testing.T) {
	r := mustGenerate(t)
	cert := parseCert(t, r)
	want := "renotify-dn_TESTDAEMON01"
	if cert.Subject.CommonName != want {
		t.Errorf("CN = %q, want %q", cert.Subject.CommonName, want)
	}
}

func TestGenerate_SANs(t *testing.T) {
	r := mustGenerate(t)
	cert := parseCert(t, r)

	// Check IPs.
	wantIPs := []string{"192.168.1.42", "127.0.0.1"}
	for _, want := range wantIPs {
		found := false
		for _, ip := range cert.IPAddresses {
			if ip.String() == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("SAN missing IP %s", want)
		}
	}

	// Check DNS names.
	found := false
	for _, name := range cert.DNSNames {
		if name == "localhost" {
			found = true
			break
		}
	}
	if !found {
		t.Error("SAN missing DNS name 'localhost'")
	}
}

func TestGenerate_Serial(t *testing.T) {
	r := mustGenerate(t)
	cert := parseCert(t, r)

	if cert.SerialNumber.Sign() <= 0 {
		t.Error("serial number is not positive")
	}
	// 128-bit max = 16 bytes.
	if cert.SerialNumber.BitLen() > 128 {
		t.Errorf("serial has %d bits, want <= 128",
			cert.SerialNumber.BitLen())
	}
}

func TestGenerate_SelfSigned(t *testing.T) {
	r := mustGenerate(t)
	cert := parseCert(t, r)

	if cert.Issuer.CommonName != cert.Subject.CommonName {
		t.Errorf("issuer CN %q != subject CN %q",
			cert.Issuer.CommonName, cert.Subject.CommonName)
	}

	// Verify the cert is self-signed.
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	_, err := cert.Verify(x509.VerifyOptions{
		Roots: roots,
	})
	if err != nil {
		t.Errorf("self-signed verification failed: %v", err)
	}
}

func TestGenerate_DERMatchesPEM(t *testing.T) {
	r := mustGenerate(t)
	block, _ := pem.Decode(r.CertPEM)
	if block == nil {
		t.Fatal("no PEM block in cert")
	}
	if string(block.Bytes) != string(r.CertDER) {
		t.Error("CertDER does not match PEM-decoded DER")
	}
}

func TestGenerate_FingerprintFormat(t *testing.T) {
	r := mustGenerate(t)
	if len(r.Fingerprint) != 64 {
		t.Errorf("fingerprint length = %d, want 64", len(r.Fingerprint))
	}
	for _, c := range r.Fingerprint {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("fingerprint contains non-hex char %q", c)
			break
		}
	}
}

func TestGenerate_FingerprintDeterministic(t *testing.T) {
	r := mustGenerate(t)
	fp1 := Fingerprint(r.CertDER)
	fp2 := Fingerprint(r.CertDER)
	if fp1 != fp2 {
		t.Errorf("fingerprints differ: %s vs %s", fp1, fp2)
	}
}

func TestFingerprint_KnownVector(t *testing.T) {
	// SHA-256 of empty input is well-known.
	data := []byte{}
	h := sha256.Sum256(data)
	want := hex.EncodeToString(h[:])
	got := Fingerprint(data)
	if got != want {
		t.Errorf("Fingerprint(empty) = %s, want %s", got, want)
	}
}

func TestFingerprintFromPEMFile_Valid(t *testing.T) {
	r := mustGenerate(t)

	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	os.WriteFile(certPath, r.CertPEM, 0644)

	fp, err := FingerprintFromPEMFile(certPath)
	if err != nil {
		t.Fatalf("FingerprintFromPEMFile: %v", err)
	}
	if fp != r.Fingerprint {
		t.Errorf("file fingerprint %s != in-memory %s", fp, r.Fingerprint)
	}
}

func TestFingerprintFromPEMFile_Missing(t *testing.T) {
	_, err := FingerprintFromPEMFile("/nonexistent/cert.pem")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFingerprintFromPEMFile_InvalidPEM(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pem")
	os.WriteFile(path, []byte("not a pem file"), 0644)

	_, err := FingerprintFromPEMFile(path)
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestGenerate_KeyUsage(t *testing.T) {
	r := mustGenerate(t)
	cert := parseCert(t, r)

	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Error("KeyUsage missing DigitalSignature")
	}

	found := false
	for _, usage := range cert.ExtKeyUsage {
		if usage == x509.ExtKeyUsageServerAuth {
			found = true
			break
		}
	}
	if !found {
		t.Error("ExtKeyUsage missing ServerAuth")
	}
}

func TestGenerate_UniqueSerial(t *testing.T) {
	r1 := mustGenerate(t)
	r2 := mustGenerate(t)
	c1 := parseCert(t, r1)
	c2 := parseCert(t, r2)
	if c1.SerialNumber.Cmp(c2.SerialNumber) == 0 {
		t.Error("two certificates have the same serial number")
	}
}

func TestGenerate_EmptyParams(t *testing.T) {
	r, err := Generate(GenerateParams{DaemonID: "test"})
	if err != nil {
		t.Fatalf("Generate with empty SANs: %v", err)
	}
	cert := parseCert(t, r)
	if len(cert.IPAddresses) != 0 {
		t.Errorf("expected 0 IPs, got %d", len(cert.IPAddresses))
	}
}

func TestGenerate_ECDSAPublicKey(t *testing.T) {
	r := mustGenerate(t)
	cert := parseCert(t, r)
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatal("public key is not ECDSA")
	}
	if pub.Curve != elliptic.P256() {
		t.Errorf("public key curve = %v, want P-256",
			pub.Curve.Params().Name)
	}
}

func TestGenerate_BigIntSerial(t *testing.T) {
	// Verify serial fits in 128 bits.
	r := mustGenerate(t)
	cert := parseCert(t, r)
	max := new(big.Int).Lsh(big.NewInt(1), 128)
	if cert.SerialNumber.Cmp(max) >= 0 {
		t.Error("serial >= 2^128")
	}
}
