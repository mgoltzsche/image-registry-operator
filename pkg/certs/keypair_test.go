package certs

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCertGen(t *testing.T) {
	ttl := 9 * time.Second
	caTTL = ttl
	certTTL = ttl

	startTime := time.Now()
	mockTimeNow(startTime)
	ca, err := NewSelfSignedCAKeyPair("registry.operator.fake.root")
	require.NoError(t, err, "NewSelfSignedCA")
	require.True(t, startTime.Add(8*time.Second).Before(ca.x509Cert.NotAfter) && startTime.Add(ttl).Add(time.Second).After(ca.x509Cert.NotAfter), "ca.notAfter")
	require.False(t, ca.NeedsRenewal(), "CA shouldn't need renewal after initialization")
	mockTimeNow(startTime.Add(6 * time.Second))
	require.False(t, ca.NeedsRenewal(), "CA shouldn't need renewal after 6s")
	mockTimeNow(startTime.Add(7 * time.Second))
	require.True(t, ca.NeedsRenewal(), "CA needs renewal after 7s (TTL: 9s)")
	parsed, err := X509KeyPair(ca.KeyPEM(), ca.CertPEM(), ca.CACertPEM())
	require.NoError(t, err, "X509KeyPair(ca)")
	require.NotNil(t, parsed, "X509KeyPair(ca)")
	_, err = tls.X509KeyPair(ca.CertPEM(), ca.KeyPEM())
	require.NoError(t, err, "tls.X509KeyPair")

	cert, err := NewServerKeyPair([]string{"localhost"}, ca)
	require.NoError(t, err, "NewServerKeyPair")
	mockTimeNow(startTime)
	require.False(t, cert.NeedsRenewal(), "cert needs renewal after initialization")
	mockTimeNow(startTime.Add(time.Second))
	require.False(t, cert.NeedsRenewal(), "cert needs renewal after 1s")
	svcCert, err := tls.X509KeyPair(cert.CertPEM(), cert.KeyPEM())
	require.NoError(t, err, "tls.X509KeyPair")
	parsed, err = X509KeyPair(cert.KeyPEM(), cert.CertPEM(), cert.CACertPEM())
	require.NoError(t, err, "X509KeyPair(svcCert)")
	require.NotNil(t, parsed, "X509KeyPair(svcCert)")
	svcCert, err = tls.X509KeyPair(parsed.CertPEM(), parsed.KeyPEM())
	require.NoError(t, err, "tls.X509KeyPair(X509KeyPair())")
	serverTLSConf := &tls.Config{
		Certificates: []tls.Certificate{svcCert},
	}

	certpool := x509.NewCertPool()
	certpool.AppendCertsFromPEM(ca.CertPEM())
	clientTLSConf := &tls.Config{
		RootCAs: certpool,
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "success")
	}))
	server.TLS = serverTLSConf
	server.StartTLS()
	defer server.Close()

	transport := &http.Transport{
		TLSClientConfig: clientTLSConf,
	}
	http := http.Client{
		Transport: transport,
	}
	_, err = http.Get("http://localhost" + regexp.MustCompile(":[0-9]+$").FindString(server.URL))
	require.NoError(t, err, "HTTP GET request on test server using TLS cert")
}

func mockTimeNow(now time.Time) {
	timeNow = func() time.Time { return now }
}
