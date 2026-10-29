package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/fleet"
)

type fleetFixture struct {
	node               fleet.Node
	client, server, ca testCertificate
	path               string
	calls              *atomic.Int32
	tls                *httptest.Server
}

func newFleetFixture(t *testing.T, id string) fleetFixture {
	t.Helper()
	ca := issueTestCertificate(t, nil, "ca", false)
	sv := issueTestCertificate(t, &ca, "server", false)
	client := issueTestCertificate(t, &ca, "client", false)
	path := filepath.Join(t.TempDir(), "snapshot.json")
	snapshot := fleet.Snapshot{Schema: 1, NodeID: id, GeneratedAt: time.Now(), Version: "test", Projects: []fleet.Project{{Name: "private-project", RegistryStatus: "running"}}, Monitoring: fleet.Checks{Status: "unknown"}, Security: fleet.Security{Status: "unknown"}}
	b, _ := json.Marshal(snapshot)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	allowed := map[[32]byte]bool{sha256.Sum256(client.cert.Raw): true}
	handler := fleet.Agent(id, path, allowed)
	calls := &atomic.Int32{}
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); handler.ServeHTTP(w, r) }))
	tlsConfig, err := (ListenerOptions{CertFile: sv.certFile, KeyFile: sv.keyFile, ClientCAFile: ca.certFile, AllowedClients: []string{client.certFile}}).TLSConfig(s.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	s.TLS = tlsConfig
	s.StartTLS()
	t.Cleanup(s.Close)
	node := fleet.Node{ID: id, Label: id, Group: "demo", URL: s.URL, Address: s.Listener.Addr().String(), CAFile: ca.certFile, ServerCertFile: sv.certFile, ClientCertFile: client.certFile, ClientKeyFile: client.keyFile, ViewGroups: []string{"view-" + id}, DetailGroups: []string{"detail-" + id}}
	return fleetFixture{node, client, sv, ca, path, calls, s}
}

func TestFleetTransportAuthenticationAndFreshness(t *testing.T) {
	f := newFleetFixture(t, "alpha")
	for _, tc := range []struct {
		name   string
		change func(*fleet.Node)
		want   bool
	}{
		{"valid", func(*fleet.Node) {}, true},
		{"sibling server certificate", func(n *fleet.Node) {
			c := issueTestCertificate(t, &f.ca, "server", false)
			n.ServerCertFile = c.certFile
		}, false},
		{"wrong CA", func(n *fleet.Node) { c := issueTestCertificate(t, nil, "ca", false); n.CAFile = c.certFile }, false},
		{"unenrolled client", func(n *fleet.Node) {
			c := issueTestCertificate(t, &f.ca, "client", false)
			n.ClientCertFile = c.certFile
			n.ClientKeyFile = c.keyFile
		}, false},
		{"expired client", func(n *fleet.Node) {
			c := issueTestCertificate(t, &f.ca, "client", true)
			n.ClientCertFile = c.certFile
			n.ClientKeyFile = c.keyFile
		}, false},
		{"node substitution", func(n *fleet.Node) { n.ID = "beta" }, false},
		{"wrong hostname", func(n *fleet.Node) { n.URL = strings.Replace(n.URL, "127.0.0.1", "wrong.invalid", 1) }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := f.node
			tc.change(&n)
			clients, err := fleet.NewClients(fleet.Config{Enabled: true, Servers: []fleet.Node{n}})
			if err != nil {
				t.Fatal(err)
			}
			_, err = clients[n.ID].Fetch(context.Background())
			if (err == nil) != tc.want {
				t.Fatalf("success=%v want %v", err == nil, tc.want)
			}
		})
	}
	b, _ := os.ReadFile(f.path)
	var s fleet.Snapshot
	json.Unmarshal(b, &s)
	s.GeneratedAt = time.Now().Add(-time.Hour)
	b, _ = json.Marshal(s)
	os.WriteFile(f.path, b, 0600)
	clients, _ := fleet.NewClients(fleet.Config{Enabled: true, Servers: []fleet.Node{f.node}})
	if _, err := clients["alpha"].Fetch(context.Background()); err == nil {
		t.Fatal("stale export accepted")
	}
}

func TestFleetAgentHasNoCommandRoutes(t *testing.T) {
	f := newFleetFixture(t, "alpha")
	pool := x509.NewCertPool()
	pool.AddCert(f.ca.cert)
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{f.client.pair}, MinVersion: tls.VersionTLS13}}}
	for _, tc := range []struct {
		method, path string
		want         int
	}{{"GET", "/v1/snapshot", 200}, {"POST", "/v1/snapshot", 405}, {"GET", "/v1/snapshot?path=/etc/passwd", 404}, {"POST", "/api/projects/app/deploy", 404}, {"GET", "/metrics", 404}} {
		r, _ := http.NewRequest(tc.method, f.tls.URL+tc.path, nil)
		response, err := c.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != tc.want {
			t.Fatalf("%s: got %d", tc.path, response.StatusCode)
		}
	}
}

func TestFleetRejectsUntrustedEnrollment(t *testing.T) {
	f := newFleetFixture(t, "alpha")
	for _, change := range []func(*fleet.Node){func(n *fleet.Node) { n.URL = "http://127.0.0.1" }, func(n *fleet.Node) { n.Address = "169.254.169.254:80" }, func(n *fleet.Node) { n.Address = "example.invalid:443" }, func(n *fleet.Node) { n.Address = "8.8.8.8:443" }, func(n *fleet.Node) { n.ViewGroups = nil; n.DetailGroups = nil }, func(n *fleet.Node) { n.URL += "/proxy" }} {
		n := f.node
		change(&n)
		if (fleet.Config{Enabled: true, Servers: []fleet.Node{n}}).Validate() == nil {
			t.Fatal("unsafe enrollment accepted")
		}
	}
	n := f.node
	n.ID = "beta"
	if _, err := fleet.NewClients(fleet.Config{Enabled: true, Servers: []fleet.Node{f.node, n}}); err == nil {
		t.Fatal("shared node client key accepted")
	}
}

func TestFleetServerPermissionsAndLegacyRouteIsolation(t *testing.T) {
	a, b := newFleetFixture(t, "alpha"), newFleetFixture(t, "beta")
	cfg := config.DefaultConfig()
	cfg.Fleet = fleet.Config{Enabled: true, Servers: []fleet.Node{a.node, b.node}}
	cfg.Server.OIDC.Enabled = true
	s := New(cfg, nil, "127.0.0.1:8420")
	defer s.Shutdown(context.Background())
	s.oidc = &oidcLogin{origin: &url.URL{Scheme: "https", Host: "localhost:8420"}}
	var err error
	s.fleetClients, err = fleet.NewClients(cfg.Fleet)
	if err != nil {
		t.Fatal(err)
	}
	device := issueTestCertificate(t, &a.ca, "client", false)
	peer := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{device.cert}, VerifiedChains: [][]*x509.Certificate{{device.cert, a.ca.cert}}}
	groups := []string{"view-alpha"}
	token, _ := s.sessions.createIdentityGroups(fmtDevice(device), "reader", groups, time.Now().Add(time.Minute))
	groups[0] = "detail-beta"
	request := func(method, path, token string, peer *tls.ConnectionState) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "https://localhost:8420"+path, nil)
		r.TLS = peer
		r.Header.Set("Origin", "https://localhost:8420")
		r.Header.Set("Remote-Groups", "detail-beta")
		r.AddCookie(&http.Cookie{Name: s.sessionCookieName(), Value: token})
		w := httptest.NewRecorder()
		s.server.Handler.ServeHTTP(w, r)
		return w
	}
	w := request("GET", "/api/fleet", token, peer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "alpha") || strings.Contains(w.Body.String(), "beta") || strings.Contains(w.Body.String(), "private-project") {
		t.Fatalf("permission leak: %d %s", w.Code, w.Body.String())
	}
	if a.calls.Load() != 1 || b.calls.Load() != 0 {
		t.Fatal("unauthorized node contacted")
	}
	for _, path := range []string{"/api/fleet/alpha", "/api/fleet/beta", "/api/fleet/missing"} {
		if request("GET", path, token, peer).Code != 404 {
			t.Fatal("detail access not denied uniformly")
		}
	}
	for _, path := range []string{"/api/projects", "/api/config", "/api/servers", "/metrics", "/api/webhook/deploy/app"} {
		if request("POST", path, token, peer).Code < 400 {
			t.Fatal("legacy route exposed")
		}
	}
	if request("GET", "/api/fleet", token, nil).Code < 400 {
		t.Fatal("session moved off device")
	}
	detail, _ := s.sessions.createIdentityGroups(fmtDevice(device), "operator", []string{"detail-alpha"}, time.Now().Add(time.Minute))
	w = request("GET", "/api/fleet/alpha", detail, peer)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "private-project") {
		t.Fatal("permitted detail missing")
	}
	s.sessions.revoke(detail)
	if request("GET", "/api/fleet/alpha", detail, peer).Code != 401 {
		t.Fatal("revoked session accepted")
	}
}

func fmtDevice(c testCertificate) string {
	h := sha256.Sum256(c.cert.Raw)
	const hex = "0123456789abcdef"
	b := make([]byte, 64)
	for i, v := range h {
		b[2*i] = hex[v>>4]
		b[2*i+1] = hex[v&15]
	}
	return string(b)
}

func TestFleetRequiresLocalDeviceAndPasskey(t *testing.T) {
	c := config.DefaultConfig()
	c.Fleet.Enabled = true
	c.Server.OIDC = config.OIDCConfig{Enabled: true, PublicURL: "https://localhost:8420", AuthenticationMethod: "webauthn"}
	good := &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, VerifyConnection: func(tls.ConnectionState) error { return nil }}
	if err := ValidateFleetMode(c, "127.0.0.1:8420", good); err != nil {
		t.Fatal(err)
	}
	for _, addr := range []string{"0.0.0.0:8420", "[::]:8420", "10.0.0.1:8420", "localhost:8420"} {
		if ValidateFleetMode(c, addr, good) == nil {
			t.Fatal("non-loopback bind accepted")
		}
	}
	if ValidateFleetMode(c, "127.0.0.1:8420", nil) == nil {
		t.Fatal("device TLS bypass")
	}
	c.Server.OIDC.AuthenticationMethod = "mfa"
	if ValidateFleetMode(c, "127.0.0.1:8420", good) == nil {
		t.Fatal("passkey policy downgrade")
	}
	c.Server.OIDC.AuthenticationMethod = "webauthn"
	c.Server.OIDC.PublicURL = "https://public.example.invalid"
	if ValidateFleetMode(c, "127.0.0.1:8420", good) == nil {
		t.Fatal("public origin accepted")
	}
}
