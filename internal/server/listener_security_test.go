package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type testCertificate struct {
	cert              *x509.Certificate
	key               *ecdsa.PrivateKey
	pair              tls.Certificate
	certFile, keyFile string
}

// Every test creates ephemeral keys; no fixed private key is checked in.
func issueTestCertificate(t *testing.T, ca *testCertificate, kind string, expired bool) testCertificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "test-only"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	if expired {
		template.NotBefore = time.Now().Add(-2 * time.Hour)
		template.NotAfter = time.Now().Add(-time.Hour)
	}
	switch kind {
	case "ca":
		template.IsCA = true
		template.BasicConstraintsValid = true
		template.KeyUsage |= x509.KeyUsageCertSign
	case "server":
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	case "client":
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	parent, signer := template, key
	if ca != nil {
		parent, signer = ca.cert, ca.key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return testCertificate{parsed, key, pair, certFile, keyFile}
}

func TestDashboardListenerRejectsExposureAndPartialTLS(t *testing.T) {
	for _, addr := range []string{":8420", "0.0.0.0:8420", "[::]:8420", "192.0.2.1:8420", "localhost:8420", "bad"} {
		if _, err := (ListenerOptions{}).TLSConfig(addr); err == nil {
			t.Errorf("accepted cleartext listener %s", addr)
		}
	}
	for _, addr := range []string{"127.0.0.1:8420", "[::1]:8420"} {
		if config, err := (ListenerOptions{}).TLSConfig(addr); err != nil || config != nil {
			t.Fatalf("loopback: %v", err)
		}
		if _, err := (ListenerOptions{CertFile: "missing.pem"}).TLSConfig(addr); err == nil {
			t.Fatal("partial TLS fell back to HTTP")
		}
	}
	srv, _ := setupAuthTestServer(t)
	srv.apiToken = strings.Repeat("x", 32)
	if err := srv.Start(); err == nil {
		t.Fatal("direct Start bypassed listener policy")
	}
}

func TestDashboardDeviceAuthenticationOverTLS(t *testing.T) {
	ca := issueTestCertificate(t, nil, "ca", false)
	serverCert := issueTestCertificate(t, &ca, "server", false)
	device := issueTestCertificate(t, &ca, "client", false)
	other := issueTestCertificate(t, &ca, "client", false)
	expired := issueTestCertificate(t, &ca, "client", true)
	wrongUsage := issueTestCertificate(t, &ca, "server", false)
	untrustedCA := issueTestCertificate(t, nil, "ca", false)
	untrusted := issueTestCertificate(t, &untrustedCA, "client", false)
	options := ListenerOptions{CertFile: serverCert.certFile, KeyFile: serverCert.keyFile, ClientCAFile: ca.certFile, AllowedClients: []string{device.certFile, expired.certFile, wrongUsage.certFile, untrusted.certFile}}
	config, err := options.TLSConfig("0.0.0.0:8420")
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := setupAuthTestServer(t)
	https := httptest.NewUnstartedServer(srv.server.Handler)
	https.Config.ErrorLog = log.New(io.Discard, "", 0)
	https.TLS = config
	https.StartTLS()
	defer https.Close()
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)
	for _, tc := range []struct {
		name        string
		certificate *testCertificate
		token       bool
		want        int
	}{
		{"no device even with token", nil, true, 0},
		{"trusted CA but unlisted device", &other, true, 0},
		{"listed but expired", &expired, true, 0},
		{"listed but wrong usage", &wrongUsage, true, 0},
		{"listed but untrusted", &untrusted, true, 0},
		{"device alone", &device, false, http.StatusUnauthorized},
		{"device and token", &device, true, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientTLS := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}
			if tc.certificate != nil {
				// Force sending even a wrong-issuer certificate to test server validation.
				clientTLS.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &tc.certificate.pair, nil }
			}
			transport := &http.Transport{TLSClientConfig: clientTLS}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			request, _ := http.NewRequest("GET", https.URL+"/api/projects", nil)
			request.Header.Set("X-Client-Cert", "forged")
			request.Header.Set("X-Forwarded-Proto", "https")
			if tc.token {
				request.Header.Set("Authorization", "Bearer "+srv.apiToken)
			}
			response, err := client.Do(request)
			if tc.want == 0 {
				if err == nil {
					response.Body.Close()
					t.Fatal("unauthorized device reached HTTP")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tc.want {
				t.Fatalf("status %d, want %d", response.StatusCode, tc.want)
			}
		})
	}
	// Restart with a different device list revokes the previous certificate.
	options.AllowedClients = []string{other.certFile}
	reconfigured, err := options.TLSConfig("127.0.0.1:8420")
	if err != nil {
		t.Fatal(err)
	}
	if err := reconfigured.VerifyConnection(tls.ConnectionState{PeerCertificates: []*x509.Certificate{device.cert}, VerifiedChains: [][]*x509.Certificate{{device.cert, ca.cert}}}); err == nil {
		t.Fatal("removed device still authorized")
	}
}

func TestBrowserSessionCannotMoveBetweenDevices(t *testing.T) {
	ca := issueTestCertificate(t, nil, "ca", false)
	a := issueTestCertificate(t, &ca, "client", false)
	b := issueTestCertificate(t, &ca, "client", false)
	srv, _ := setupAuthTestServer(t)
	state := func(cert testCertificate) *tls.ConnectionState {
		return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert.cert}, VerifiedChains: [][]*x509.Certificate{{cert.cert, ca.cert}}}
	}
	login := httptest.NewRequest("POST", "https://admin.example.invalid/login", strings.NewReader(url.Values{"token": {srv.apiToken}}.Encode()))
	login.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	login.Header.Set("Origin", "https://admin.example.invalid")
	login.TLS = state(a)
	w := httptest.NewRecorder()
	srv.handleLoginSubmit(w, login)
	if w.Code != http.StatusFound {
		t.Fatalf("login status %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Fatal("missing secure browser session")
	}
	for _, tc := range []struct {
		name string
		tls  *tls.ConnectionState
		want int
	}{
		{"original device", state(a), http.StatusOK},
		{"another authorized device", state(b), http.StatusUnauthorized},
		{"no verified certificate", &tls.ConnectionState{}, http.StatusUnauthorized},
		{"original remains valid after rejected replay", state(a), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://admin.example.invalid/api/projects", nil)
			r.TLS = tc.tls
			r.AddCookie(cookies[0])
			r.Header.Set("X-Client-Cert", requestDevice(login))
			w := httptest.NewRecorder()
			srv.server.Handler.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status %d, want %d", w.Code, tc.want)
			}
		})
	}
}
