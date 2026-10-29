// Package fleet implements a read-only, explicitly enrolled server overview.
// It never opens SSH connections, invokes Docker or accepts remote commands.
package fleet

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"
)

const MaxBytes = 1 << 20
const MaxAge = 3 * time.Minute

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type Project struct {
	Name           string     `json:"name"`
	RegistryStatus string     `json:"registry_status"`
	LastBackup     *time.Time `json:"last_backup,omitempty"`
}

type Checks struct {
	Status    string     `json:"status"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
	Healthy   int        `json:"healthy"`
	Unhealthy int        `json:"unhealthy"`
	Unknown   int        `json:"unknown"`
}

type Security struct {
	Status          string     `json:"status"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
	Critical        int        `json:"critical"`
	High            int        `json:"high"`
	RuntimeFindings int        `json:"runtime_findings"`
	Unknown         int        `json:"unknown"`
}

// Only deliberately selected fields cross the node boundary. No URLs, file
// paths, environment, logs, identities, tokens or raw scanner errors belong here.
type Snapshot struct {
	Schema      int       `json:"schema"`
	NodeID      string    `json:"node_id"`
	GeneratedAt time.Time `json:"generated_at"`
	Version     string    `json:"version"`
	Commit      string    `json:"commit"`
	Projects    []Project `json:"projects"`
	Monitoring  Checks    `json:"monitoring"`
	Security    Security  `json:"security"`
}

func ValidID(id string) bool { return identifier.MatchString(id) }

func (s Snapshot) Validate(node string, now time.Time) error {
	if s.Schema != 1 || !ValidID(node) || s.NodeID != node || len(s.Version) > 100 || len(s.Commit) > 100 || len(s.Projects) > 500 {
		return fmt.Errorf("invalid snapshot identity or limits")
	}
	if s.GeneratedAt.IsZero() || s.GeneratedAt.After(now.Add(30*time.Second)) || now.Sub(s.GeneratedAt) > MaxAge {
		return fmt.Errorf("snapshot is stale or has an invalid clock")
	}
	seen := map[string]bool{}
	for _, p := range s.Projects {
		if !ValidID(p.Name) || seen[p.Name] || len(p.RegistryStatus) > 40 || (p.LastBackup != nil && p.LastBackup.After(now.Add(30*time.Second))) {
			return fmt.Errorf("invalid project summary")
		}
		seen[p.Name] = true
	}
	for _, status := range []string{s.Monitoring.Status, s.Security.Status} {
		if status != "ok" && status != "attention" && status != "unknown" {
			return fmt.Errorf("invalid observation status")
		}
	}
	for _, n := range []int{s.Monitoring.Healthy, s.Monitoring.Unhealthy, s.Monitoring.Unknown, s.Security.Critical, s.Security.High, s.Security.RuntimeFindings, s.Security.Unknown} {
		if n < 0 || n > 1000000 {
			return fmt.Errorf("invalid observation count")
		}
	}
	// A fresh export cannot make old monitoring or scan observations fresh.
	for _, c := range []struct {
		status string
		at     *time.Time
		age    time.Duration
	}{{s.Monitoring.Status, s.Monitoring.CheckedAt, MaxAge}, {s.Security.Status, s.Security.CheckedAt, 26 * time.Hour}} {
		if c.status != "unknown" && (c.at == nil || c.at.IsZero() || c.at.After(now.Add(30*time.Second)) || now.Sub(*c.at) > c.age) {
			return fmt.Errorf("stale observation marked current")
		}
	}
	if s.Monitoring.Status == "ok" && (s.Monitoring.Healthy == 0 || s.Monitoring.Unhealthy != 0 || s.Monitoring.Unknown != 0) {
		return fmt.Errorf("invalid healthy monitoring claim")
	}
	if s.Security.Status == "ok" && (s.Security.Critical != 0 || s.Security.High != 0 || s.Security.RuntimeFindings != 0 || s.Security.Unknown != 0) {
		return fmt.Errorf("invalid clean security claim")
	}
	return nil
}

func Decode(r io.Reader, node string, now time.Time) (Snapshot, error) {
	var s Snapshot
	b, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil || len(b) > MaxBytes {
		return s, fmt.Errorf("snapshot exceeds limits")
	}
	if err = json.Unmarshal(b, &s); err != nil {
		return s, fmt.Errorf("invalid snapshot JSON")
	}
	return s, s.Validate(node, now)
}
