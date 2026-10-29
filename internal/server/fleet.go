package server

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/audit"
	"github.com/fleetdeck/fleetdeck/internal/config"
	"github.com/fleetdeck/fleetdeck/internal/fleet"
)

// The central panel is intentionally device-local, even with valid TLS. There
// is no flag to bind it publicly and no legacy token/password fallback.
func ValidateFleetMode(c *config.Config, addr string, t *tls.Config) error {
	if !c.Fleet.Enabled {
		return nil
	}
	host, _, err := net.SplitHostPort(addr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("central fleet dashboard must bind a literal loopback address")
	}
	if t == nil || t.ClientAuth != tls.RequireAndVerifyClientCert || t.VerifyConnection == nil {
		return fmt.Errorf("central fleet dashboard requires TLS and an allowed device certificate")
	}
	if !c.Server.OIDC.Enabled || c.Server.OIDC.AuthenticationMethod == "mfa" {
		return fmt.Errorf("central fleet dashboard requires OIDC with password and WebAuthn")
	}
	u, err := url.Parse(c.Server.OIDC.PublicURL)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "localhost" && !net.ParseIP(u.Hostname()).IsLoopback()) {
		return fmt.Errorf("central fleet OIDC public_url must use HTTPS on localhost or loopback")
	}
	return c.Fleet.Validate()
}

func (s *Server) fleetIdentity(w http.ResponseWriter, r *http.Request) (sessionEntry, bool) {
	if !s.cfg.Fleet.Enabled || !s.cfg.Server.OIDC.Enabled || requestDevice(r) == "" {
		writeError(w, 403, "a device-bound OIDC session is required")
		return sessionEntry{}, false
	}
	c, err := r.Cookie(s.sessionCookieName())
	if err != nil {
		writeError(w, 401, "sign in again")
		return sessionEntry{}, false
	}
	identity, ok := s.sessions.identity(c.Value, requestDevice(r))
	if !ok {
		writeError(w, 401, "sign in again")
		return sessionEntry{}, false
	}
	return identity, true
}

type fleetRow struct {
	ID         string          `json:"id"`
	Label      string          `json:"label"`
	Group      string          `json:"group"`
	Status     string          `json:"status"`
	Details    bool            `json:"details"`
	ObservedAt *time.Time      `json:"observed_at,omitempty"`
	Version    string          `json:"version,omitempty"`
	Projects   int             `json:"projects"`
	Monitoring *fleet.Checks   `json:"monitoring,omitempty"`
	Security   *fleet.Security `json:"security,omitempty"`
}

func (s *Server) fetchFleet(ctx context.Context, n fleet.Node) (fleet.Snapshot, error) {
	select {
	case s.fleetSlots <- struct{}{}:
	case <-ctx.Done():
		return fleet.Snapshot{}, ctx.Err()
	}
	defer func() { <-s.fleetSlots }()
	c := s.fleetClients[n.ID]
	if c == nil {
		return fleet.Snapshot{}, fmt.Errorf("not configured")
	}
	snapshot, err := c.Fetch(ctx)
	if err == nil {
		err = snapshot.Validate(n.ID, time.Now())
	}
	return snapshot, err
}

func (s *Server) handleFleet(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.fleetIdentity(w, r)
	if !ok {
		return
	}
	rows := []fleetRow{}
	nodes := []fleet.Node{}
	for _, n := range s.cfg.Fleet.Servers {
		if n.CanView(identity.groups) {
			nodes = append(nodes, n)
			rows = append(rows, fleetRow{ID: n.ID, Label: n.Label, Group: n.Group, Status: "unknown", Details: n.CanDetail(identity.groups)})
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot, err := s.fetchFleet(ctx, n)
			if err != nil {
				return
			}
			rows[i].Status = "observed"
			rows[i].ObservedAt = &snapshot.GeneratedAt
			rows[i].Version = snapshot.Version
			rows[i].Projects = len(snapshot.Projects)
			rows[i].Monitoring = &snapshot.Monitoring
			rows[i].Security = &snapshot.Security
		}()
	}
	wg.Wait()
	audit.Log("fleet.overview", "", fmt.Sprintf("actor=%x", sha256.Sum256([]byte(identity.subject))), true)
	writeJSON(w, map[string]any{"servers": rows, "read_only": true, "local_only": true})
}

func (s *Server) handleFleetDetail(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.fleetIdentity(w, r)
	if !ok {
		return
	}
	for _, n := range s.cfg.Fleet.Servers {
		if n.ID == r.PathValue("node") && n.CanDetail(identity.groups) {
			ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
			defer cancel()
			snapshot, err := s.fetchFleet(ctx, n)
			audit.Log("fleet.details", n.ID, fmt.Sprintf("actor=%x", sha256.Sum256([]byte(identity.subject))), err == nil)
			if err != nil {
				writeError(w, 503, "current node state unavailable")
				return
			}
			writeJSON(w, snapshot)
			return
		}
	}
	audit.Log("fleet.denied", "", fmt.Sprintf("actor=%x", sha256.Sum256([]byte(identity.subject))), false)
	// Do not reveal whether an unauthorized node exists.
	writeError(w, 404, "server not found")
}
