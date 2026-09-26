package maxbot

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"net/http"
	"strings"
	"testing"
)

// The embedded root is exactly the Минцифры certificate from Госуслуги: nobody can
// swap the file without changing this fingerprint.
func TestEmbeddedRussianTrustedRootCA(t *testing.T) {
	block, _ := pem.Decode(russianTrustedRootCA)
	if block == nil {
		t.Fatal("the embedded file is not PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(cert.Raw)
	const want = "d26d2d0231b7c39f92cc738512ba54103519e4405d68b5bd703e9788ca8ecf31"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("SHA-256 of the root = %s, want %s", got, want)
	}
	if !cert.IsCA || cert.Subject.CommonName != "Russian Trusted Root CA" {
		t.Fatalf("subject = %s, CA = %v", cert.Subject, cert.IsCA)
	}
}

func TestHTTPClientTrustsTheRootForMAXOnly(t *testing.T) {
	client, err := HTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.RootCAs == nil ||
		transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("transport = %+v", client.Transport)
	}
	// The process-wide default transport must stay untouched.
	if def := http.DefaultTransport.(*http.Transport).TLSClientConfig; def != nil && def.RootCAs != nil {
		t.Fatal("the Russian root leaked into http.DefaultTransport")
	}
	if client.Timeout <= 30e9 {
		t.Fatalf("timeout = %s, want longer than the 30 s long poll", client.Timeout)
	}
	if !strings.Contains(string(russianTrustedRootCA), "BEGIN CERTIFICATE") {
		t.Fatal("no certificate embedded")
	}
}
