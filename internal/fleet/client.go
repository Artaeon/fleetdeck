package fleet

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Enabled bool   `toml:"enabled"`
	Servers []Node `toml:"servers"`
}

type Node struct {
	ID             string   `toml:"id"`
	Label          string   `toml:"label"`
	Group          string   `toml:"group"`
	URL            string   `toml:"url"`
	Address        string   `toml:"address"`
	CAFile         string   `toml:"ca_file"`
	ServerCertFile string   `toml:"server_cert_file"`
	ClientCertFile string   `toml:"client_cert_file"`
	ClientKeyFile  string   `toml:"client_key_file"`
	ViewGroups     []string `toml:"view_groups"`
	DetailGroups   []string `toml:"detail_groups"`
}

func (n Node) CanView(groups []string) bool {
	return matches(groups, n.ViewGroups) || n.CanDetail(groups)
}
func (n Node) CanDetail(groups []string) bool { return matches(groups, n.DetailGroups) }
func matches(actual, allowed []string) bool {
	for _, group := range actual {
		if slices.Contains(allowed, group) {
			return true
		}
	}
	return false
}

func PrivateAddress(address string) bool {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || ap.Port() == 0 || ap.Addr().Zone() != "" {
		return false
	}
	ip := ap.Addr().Unmap()
	return ip.IsPrivate() || ip.IsLoopback()
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Servers) > 64 {
		return fmt.Errorf("fleet supports at most 64 explicitly enrolled nodes")
	}
	seen := map[string]bool{}
	for _, n := range c.Servers {
		u, err := url.Parse(n.URL)
		if !ValidID(n.ID) || seen[n.ID] || n.Label == "" || len(n.Label) > 100 || len(n.Group) > 100 {
			return fmt.Errorf("invalid or duplicate fleet node")
		}
		seen[n.ID] = true
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Opaque != "" || !PrivateAddress(n.Address) {
			return fmt.Errorf("fleet node requires an HTTPS origin and pinned private IP:port")
		}
		_, port, _ := net.SplitHostPort(n.Address)
		urlPort := u.Port()
		if urlPort == "" {
			urlPort = "443"
		}
		if port != urlPort {
			return fmt.Errorf("fleet URL and pinned port must match")
		}
		if len(n.ViewGroups)+len(n.DetailGroups) == 0 || len(n.ViewGroups)+len(n.DetailGroups) > 64 {
			return fmt.Errorf("fleet node requires explicit group grants")
		}
		for _, group := range append(slices.Clone(n.ViewGroups), n.DetailGroups...) {
			if strings.TrimSpace(group) != group || group == "" || len(group) > 100 || strings.ContainsAny(group, "\r\n\x00") {
				return fmt.Errorf("invalid fleet group grant")
			}
		}
		for _, file := range []string{n.CAFile, n.ServerCertFile, n.ClientCertFile, n.ClientKeyFile} {
			if file == "" {
				return fmt.Errorf("fleet requires CA, pinned server leaf and unique client certificate/key files")
			}
		}
	}
	return nil
}

func Leaf(path string) (*x509.Certificate, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read certificate")
	}
	p, rest := pem.Decode(b)
	if p == nil || p.Type != "CERTIFICATE" || strings.TrimSpace(string(rest)) != "" {
		return nil, fmt.Errorf("expected exactly one certificate")
	}
	c, err := x509.ParseCertificate(p.Bytes)
	if err != nil || c.IsCA {
		return nil, fmt.Errorf("expected leaf certificate")
	}
	return c, nil
}

func PrivateKey(path string) error {
	i, err := os.Lstat(path)
	if err != nil || !i.Mode().IsRegular() || i.Mode().Perm()&0077 != 0 || i.Size() > 65536 {
		return fmt.Errorf("private key must be a private regular file")
	}
	return nil
}

type Client struct {
	node     Node
	http     *http.Client
	mu       sync.Mutex
	last     time.Time
	snapshot Snapshot
	err      error
}

// NewClients performs no network access. Destinations come only from trusted
// local configuration; DNS, proxies, redirects and caller URLs are not used.
func NewClients(c Config) (map[string]*Client, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	clients := map[string]*Client{}
	keys := map[[32]byte]bool{}
	if !c.Enabled {
		return clients, nil
	}
	for _, n := range c.Servers {
		if err := PrivateKey(n.ClientKeyFile); err != nil {
			return nil, err
		}
		cert, err := tls.LoadX509KeyPair(n.ClientCertFile, n.ClientKeyFile)
		if err != nil {
			return nil, fmt.Errorf("invalid fleet client credentials")
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return nil, err
		}
		key := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
		if keys[key] {
			return nil, fmt.Errorf("each node requires a different client key")
		}
		keys[key] = true
		server, err := Leaf(n.ServerCertFile)
		if err != nil {
			return nil, err
		}
		pin := sha256.Sum256(server.Raw)
		ca, err := os.ReadFile(n.CAFile)
		if err != nil {
			return nil, fmt.Errorf("cannot read fleet CA")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("invalid fleet CA")
		}
		u, _ := url.Parse(n.URL)
		transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, MaxResponseHeaderBytes: 16384, TLSHandshakeTimeout: 4 * time.Second,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 4 * time.Second}).DialContext(ctx, "tcp", n.Address)
			},
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, ServerName: u.Hostname(), Certificates: []tls.Certificate{cert}, VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.VerifiedChains) == 0 || len(cs.PeerCertificates) == 0 || sha256.Sum256(cs.PeerCertificates[0].Raw) != pin {
					return fmt.Errorf("fleet server certificate not enrolled")
				}
				return nil
			}},
		}
		clients[n.ID] = &Client{node: n, http: &http.Client{Transport: transport, Timeout: 6 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	}
	return clients, nil
}

// Serialize/cache per node to bound fan-out from concurrent browser refreshes.
// Errors replace old observations; stale success is never shown as current.
func (c *Client) Fetch(ctx context.Context) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.last) < 30*time.Second {
		return c.snapshot, c.err
	}
	c.snapshot = Snapshot{}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.node.URL+"/v1/snapshot", nil)
	if err == nil {
		var response *http.Response
		response, err = c.http.Do(req)
		if err == nil {
			defer response.Body.Close()
			if response.StatusCode != 200 {
				err = fmt.Errorf("node unavailable")
			} else {
				c.snapshot, err = Decode(response.Body, c.node.ID, time.Now())
			}
		}
	}
	c.last = time.Now()
	c.err = err
	if err != nil {
		c.snapshot = Snapshot{}
		c.err = fmt.Errorf("current node state unavailable")
	}
	return c.snapshot, c.err
}
