package fleetexport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

// Export reads recorded observations only. Registry status is not a live Docker
// probe, and a backup record is not evidence that restoration succeeded.
func Export(database *db.DB, node, version, commit, monitorPath, securityPath string, now time.Time) (fleet.Snapshot, error) {
	s := fleet.Snapshot{Schema: 1, NodeID: node, GeneratedAt: now.UTC(), Version: version, Commit: commit, Projects: []fleet.Project{}, Monitoring: fleet.Checks{Status: "unknown"}, Security: fleet.Security{Status: "unknown"}}
	projects, err := database.ListProjects()
	if err != nil {
		return s, err
	}
	for _, p := range projects {
		item := fleet.Project{Name: p.Name, RegistryStatus: p.Status}
		backups, err := database.ListBackupRecords(p.ID, 1)
		if err != nil {
			return s, err
		}
		if len(backups) > 0 {
			at := backups[0].CreatedAt
			item.LastBackup = &at
		}
		s.Projects = append(s.Projects, item)
	}
	if state, err := monitor.LoadState(monitorPath); monitorPath != "" && err == nil {
		if items, err := monitor.Summarize(state, now); err == nil && len(items) > 0 {
			m := fleet.Checks{Status: "ok", CheckedAt: &state.UpdatedAt}
			for _, item := range items {
				switch item.Status {
				case "healthy":
					m.Healthy++
				case "unhealthy":
					m.Unhealthy++
				default:
					m.Unknown++
				}
			}
			if m.Unhealthy > 0 {
				m.Status = "attention"
			}
			if m.Unknown > 0 {
				m.Status = "unknown"
			}
			if state.UpdatedAt.IsZero() || state.UpdatedAt.After(now.Add(30*time.Second)) || now.Sub(state.UpdatedAt) > fleet.MaxAge {
				m.Status = "unknown"
			}
			s.Monitoring = m
		}
	}
	if state, err := securitywatch.Load(securityPath); securityPath != "" && err == nil {
		r := state.Report
		v := fleet.Security{Status: "unknown"}
		if !r.CheckedAt.IsZero() {
			v.CheckedAt = &r.CheckedAt
		}
		if !r.CheckedAt.IsZero() && !r.CheckedAt.After(now.Add(30*time.Second)) && now.Sub(r.CheckedAt) <= 26*time.Hour {
			v.Status = "ok"
			for _, c := range []securitywatch.Check{r.Host, r.Release, r.Inventory} {
				switch c.Status {
				case "up_to_date", "checked":
				case "update_available":
					v.Status = "attention"
				default:
					v.Unknown++
				}
				if c.RebootRequired {
					v.Status = "attention"
				}
			}
			for _, im := range r.Images {
				if im.Status != "checked" {
					v.Unknown++
				}
				for _, f := range im.Findings {
					switch strings.ToUpper(f.Severity) {
					case "CRITICAL":
						v.Critical++
					case "HIGH":
						v.High++
					default:
						v.Status = "attention"
					}
				}
			}
			for _, c := range r.Containers {
				v.RuntimeFindings += len(c.Findings)
			}
			if v.Critical+v.High+v.RuntimeFindings > 0 {
				v.Status = "attention"
			}
			if v.Unknown > 0 {
				v.Status = "unknown"
			}
		}
		s.Security = v
	}
	return s, s.Validate(node, now)
}

// WriteSnapshot atomically replaces a sanitized export. The pre-created parent
// directory must be operator owned; group read permits a separate observer user.
func WriteSnapshot(path string, s fleet.Snapshot) error {
	if err := s.Validate(s.NodeID, time.Now()); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	i, err := os.Lstat(dir)
	if err != nil || !i.IsDir() || i.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("export parent must be an existing directory not writable by other users")
	}
	data, err := json.Marshal(s)
	if err != nil || len(data) > fleet.MaxBytes {
		return fmt.Errorf("snapshot exceeds limits")
	}
	f, err := os.CreateTemp(dir, ".snapshot-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(0640)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}
