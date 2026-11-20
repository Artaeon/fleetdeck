package securityreport

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/deliverystate"
)

// ControlEvidence refers to retained private evidence by SHA-256. Operators
// enroll trusted evidence files separately from a host's scanner state.
type ControlEvidence struct {
	Host      string    `json:"host"`
	Control   string    `json:"control"`
	Source    string    `json:"source"`
	Observed  time.Time `json:"observed_at"`
	Expires   time.Time `json:"expires_at"`
	Result    string    `json:"result"`
	Reference string    `json:"sha256"`
}
type controlFile struct {
	Version int               `json:"version"`
	Records []ControlEvidence `json:"records"`
}

var controlNames = []struct {
	key, en, de string
	maxAge      time.Duration
}{
	{"ssh_mfa", "SSH and multifactor login", "SSH und Mehrfaktoranmeldung", 30 * 24 * time.Hour},
	{"reachability", "External reachability", "Externe Erreichbarkeit", 26 * time.Hour},
	{"offsite_backup", "Offsite backup", "Externe Sicherung", 48 * time.Hour},
	{"restore_test", "Isolated restore test", "Isolierte Wiederherstellung", 90 * 24 * time.Hour},
}

func (r *Report) WithControls(path string) error { return r.LoadControls(path, "", "") }

// LoadControls authenticates the exact bytes parsed below when a separately
// pinned Ed25519 public key and a detached 64-byte signature are supplied.
func (r *Report) LoadControls(path, publicKey, signature string) error {
	data, err := deliverystate.ReadBytes(path, 256<<10)
	if err != nil {
		return fmt.Errorf("cannot read private control evidence: %w", err)
	}
	if publicKey != "" || signature != "" {
		if publicKey == "" || signature == "" {
			return fmt.Errorf("signed evidence requires both the pinned public key and signature")
		}
		keyPEM, err := deliverystate.ReadBytes(publicKey, 4096)
		if err != nil {
			return err
		}
		block, rest := pem.Decode(keyPEM)
		if block == nil || block.Type != "PUBLIC KEY" || len(rest) != 0 {
			return fmt.Errorf("invalid pinned public key")
		}
		k, err := x509.ParsePKIXPublicKey(block.Bytes)
		pub, ok := k.(ed25519.PublicKey)
		sig, readErr := deliverystate.ReadBytes(signature, ed25519.SignatureSize)
		if err != nil || !ok || readErr != nil || !ed25519.Verify(pub, data, sig) {
			return fmt.Errorf("control evidence signature verification failed")
		}
	}
	var file controlFile
	if json.Unmarshal(data, &file) != nil {
		return fmt.Errorf("invalid control evidence JSON")
	}
	if file.Version != 1 || len(file.Records) > 1024 {
		return fmt.Errorf("invalid control evidence file")
	}
	nodes := map[string]int{}
	for i, n := range r.Nodes {
		nodes[n.Label] = i
	}
	latest := map[[2]string]ControlEvidence{}
	for _, c := range file.Records {
		if _, ok := nodes[c.Host]; !ok {
			return fmt.Errorf("control evidence outside enrolled hosts")
		}
		var maxAge time.Duration
		for _, kind := range controlNames {
			if kind.key == c.Control {
				maxAge = kind.maxAge
			}
		}
		if maxAge == 0 || !identifier.MatchString(c.Source) || !validSHA256(c.Reference) || (c.Result != "pass" && c.Result != "fail" && c.Result != "unknown") || c.Observed.IsZero() || c.Expires.IsZero() || !c.Expires.After(c.Observed) || c.Expires.Sub(c.Observed) > maxAge {
			return fmt.Errorf("invalid control, source, validity interval or evidence reference")
		}
		key := [2]string{c.Host, c.Control}
		if old, ok := latest[key]; ok {
			if old.Observed.Equal(c.Observed) && old != c {
				return fmt.Errorf("conflicting evidence at the same instant")
			}
			if !c.Observed.After(old.Observed) {
				continue
			}
		}
		latest[key] = c
	}
	for i := range r.Nodes {
		n := &r.Nodes[i]
		for _, kind := range controlNames {
			c, ok := latest[[2]string{n.Label, kind.key}]
			if !ok {
				c = ControlEvidence{Host: n.Label, Control: kind.key, Result: "missing"}
			}
			if ok && (c.Observed.After(r.GeneratedAt.Add(30*time.Second)) || !r.GeneratedAt.Before(c.Expires)) {
				c.Result = "expired"
			}
			n.Controls = append(n.Controls, c)
			switch c.Result {
			case "pass":
				continue
			case "fail":
				n.Warnings++
				n.Status = "attention"
			default:
				n.Unknown++
				if n.Status == "checked" {
					n.Status = "unknown"
				}
			}
			n.Actions = append(n.Actions, "check_"+kind.key)
		}
	}
	r.NotCovered = []string{"hoster_integrity"}
	return nil
}
func validSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func controlStatus(value string, de bool) string {
	switch value {
	case "pass":
		return words(de, "Verified", "Bestätigt")
	case "fail":
		return words(de, "Failed", "Fehlgeschlagen")
	case "expired":
		return words(de, "Stale / clock invalid", "Veraltet / Zeit ungültig")
	case "unknown":
		return words(de, "Inconclusive", "Ungeklärt")
	default:
		return words(de, "Missing", "Fehlt")
	}
}
