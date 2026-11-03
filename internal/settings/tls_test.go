package settings

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExplicitTLSMustBeValid(t *testing.T) {
	dir := t.TempDir()
	cert, key := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	for _, files := range [][2]string{{cert, key}} {
		if _, err := Parse([]string{"-tls-cert", files[0], "-tls-key", files[1]}, func(string) string { return "" }); err == nil {
			t.Error("accepted missing TLS files")
		}
	}
	if err := os.WriteFile(cert, []byte("invalid cert"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("invalid key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse([]string{"-tls-cert", cert, "-tls-key", key}, func(string) string { return "" }); err == nil {
		t.Error("accepted malformed TLS files")
	}
}

func TestOptionalTLSAcceptsMatchingPair(t *testing.T) {
	cert, key := testTLSFiles(t)
	for _, envMode := range []bool{false, true} {
		var args []string
		env := map[string]string{}
		if envMode {
			env["VYOS_SFLOW_TLS_CERT"], env["VYOS_SFLOW_TLS_KEY"] = cert, key
		} else {
			args = []string{"-tls-cert", cert, "-tls-key", key}
		}
		c, err := Parse(args, func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		if c.TLSCert != cert || c.TLSKey != key {
			t.Fatalf("TLS paths not preserved: %+v", c)
		}
	}
	_, otherKey := testTLSFiles(t)
	if _, err := Parse([]string{"-tls-cert", cert, "-tls-key", otherKey}, func(string) string { return "" }); err == nil {
		t.Error("accepted mismatched TLS key")
	}
}

func testTLSFiles(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{"localhost"}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private}), 0600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}
