package server

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
)

// ListenerOptions keeps device credentials outside the public project/config.
// The client CA establishes trust; the leaf allowlist authorizes specific devices.
type ListenerOptions struct {
	CertFile, KeyFile, ClientCAFile string
	AllowedClients                  []string
}

func (o ListenerOptions) TLSConfig(addr string) (*tls.Config, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid dashboard listen address: %w", err)
	}
	enabled := o.CertFile != "" || o.KeyFile != "" || o.ClientCAFile != "" || len(o.AllowedClients) != 0
	if !enabled {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("dashboard HTTP requires a literal loopback address; remote listeners require TLS, a client CA and allowed client certificates")
		}
		return nil, nil
	}
	if o.CertFile == "" || o.KeyFile == "" || o.ClientCAFile == "" || len(o.AllowedClients) == 0 {
		return nil, fmt.Errorf("dashboard TLS requires --tls-cert, --tls-key, --client-ca and at least one --allow-client-cert")
	}
	certificate, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading dashboard TLS certificate: %w", err)
	}
	caPEM, err := os.ReadFile(o.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("reading dashboard client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("dashboard client CA contains no certificates")
	}
	allowed := make(map[[32]byte]bool)
	for _, path := range o.AllowedClients {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading allowed client certificate: %w", err)
		}
		block, rest := pem.Decode(data)
		if block == nil || block.Type != "CERTIFICATE" || strings.TrimSpace(string(rest)) != "" {
			return nil, fmt.Errorf("allowed client file must contain exactly one PEM certificate")
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || cert.IsCA {
			return nil, fmt.Errorf("allowed client must be a valid leaf certificate")
		}
		allowed[sha256.Sum256(cert.Raw)] = true
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		// Disabling resumption forces certificate validity checks on every new
		// connection. Removing a device requires restarting the listener.
		SessionTicketsDisabled: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 || !allowed[sha256.Sum256(state.PeerCertificates[0].Raw)] {
				return fmt.Errorf("dashboard client certificate is not authorized")
			}
			return nil
		},
	}, nil
}

// ConfigureListener must be called before Start. Partial TLS setup never falls
// back to HTTP, including on loopback.
func (s *Server) ConfigureListener(options ListenerOptions) error {
	config, err := options.TLSConfig(s.server.Addr)
	if err != nil {
		return err
	}
	s.server.TLSConfig = config
	return nil
}

// Only the verified TLS peer identifies a device, never proxy headers. Sessions
// created in private HTTP mode cannot be replayed through a device TLS listener.
func requestDevice(r *http.Request) string {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		return ""
	}
	return fmt.Sprintf("%x", sha256.Sum256(r.TLS.PeerCertificates[0].Raw))
}
