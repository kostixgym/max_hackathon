package maxbot

import (
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"errors"
	"net/http"
	"time"
)

// The MAX Bot API (platform-api2.max.ru) presents a certificate of the Russian national
// certificate authority: *.max.ru ← Russian Trusted Sub CA ← Russian Trusted Root CA
// (Минцифры России). The root is not in the usual trust stores — neither in the
// runtime image nor on developer machines — so without it every call fails with
// «x509: certificate signed by unknown authority».
//
// The root is trusted only by the HTTP client of the MAX API, not by the whole
// process: this authority cannot vouch for any other connection of the service.
//
// Source: https://gu-st.ru/content/lending/russian_trusted_root_ca_pem.crt (Госуслуги),
// valid until 27.02.2032, SHA-256 D2:6D:2D:02:31:B7:C3:9F:92:CC:73:85:12:BA:54:10:35:19:E4:40:5D:68:B5:BD:70:3E:97:88:CA:8E:CF:31.
//
//go:embed certs/russian_trusted_root_ca.pem
var russianTrustedRootCA []byte

// httpTimeout is longer than the 30-second long polling of GetUpdates: an empty poll
// then ends with the server's answer, not with a client timeout.
const httpTimeout = 45 * time.Second

// HTTPClient returns the HTTP client for the MAX API: the system roots plus the
// Russian Trusted Root CA.
func HTTPClient() (*http.Client, error) {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if !pool.AppendCertsFromPEM(russianTrustedRootCA) {
		return nil, errors.New("maxbot: the embedded Russian Trusted Root CA is not a valid PEM certificate")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}

	return &http.Client{Transport: transport, Timeout: httpTimeout}, nil
}
