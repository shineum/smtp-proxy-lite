package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	standardtls "crypto/tls"
	"crypto/x509"
	"testing"
	"time"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	t.Parallel()

	cert, err := GenerateSelfSignedCert()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatal("certificate is nil")
	}

	// Parse the leaf certificate to inspect it
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	// Verify CN=localhost
	if leaf.Subject.CommonName != "localhost" {
		t.Errorf("CN: got %q, want %q", leaf.Subject.CommonName, "localhost")
	}

	// Verify DNS SANs
	foundDNS := false
	for _, dns := range leaf.DNSNames {
		if dns == "localhost" {
			foundDNS = true
			break
		}
	}
	if !foundDNS {
		t.Errorf("DNS SANs: %v does not contain localhost", leaf.DNSNames)
	}

	// Verify IP SANs
	foundIP := false
	for _, ip := range leaf.IPAddresses {
		if ip.String() == "127.0.0.1" {
			foundIP = true
			break
		}
	}
	if !foundIP {
		t.Errorf("IP SANs: %v does not contain 127.0.0.1", leaf.IPAddresses)
	}

	// Verify validity period (approximately 1 year)
	validDuration := leaf.NotAfter.Sub(leaf.NotBefore)
	expectedDuration := 365 * 24 * time.Hour
	if validDuration < expectedDuration-time.Hour || validDuration > expectedDuration+time.Hour {
		t.Errorf("validity duration: got %v, want approximately %v", validDuration, expectedDuration)
	}

	// Verify the key is ECDSA P-256
	ecKey, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Fatal("public key is not ECDSA")
	}
	if ecKey.Curve != elliptic.P256() {
		t.Errorf("curve: got %v, want P-256", ecKey.Curve.Params().Name)
	}

	// Verify the certificate is self-signed
	if leaf.Issuer.CommonName != leaf.Subject.CommonName {
		t.Errorf("issuer CN %q does not match subject CN %q", leaf.Issuer.CommonName, leaf.Subject.CommonName)
	}
}

func TestLoadOrGenerateTLS_SelfSigned(t *testing.T) {
	t.Parallel()

	tlsConfig, err := LoadOrGenerateTLS("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsConfig == nil {
		t.Fatal("TLS config is nil")
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Errorf("Certificates: got %d, want 1", len(tlsConfig.Certificates))
	}
	if tlsConfig.MinVersion != standardtls.VersionTLS12 {
		t.Errorf("MinVersion: got %d, want TLS 1.2 (%d)", tlsConfig.MinVersion, standardtls.VersionTLS12)
	}
}

func TestLoadOrGenerateTLS_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadOrGenerateTLS("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("expected error for nonexistent files, got nil")
	}
}
