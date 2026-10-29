package fleetexport

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/db"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

func TestExportPreservesFindingsAndExcludesPrivateFields(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	p := &db.Project{Name: "demo", Domain: "private.example.invalid", LinuxUser: "private-user", ProjectPath: "/private/project", Status: "running", GitHubRepo: "private/repository"}
	if err := d.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	if err := d.CreateBackupRecord(&db.BackupRecord{ProjectID: p.ID, Path: "/private/backup"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	path := filepath.Join(dir, "security.json")
	state := securitywatch.State{Version: 1, Report: securitywatch.Report{CheckedAt: now, Host: securitywatch.Check{Status: "up_to_date"}, Release: securitywatch.Check{Status: "unknown", Code: "private-error"}, Inventory: securitywatch.Check{Status: "checked"}, Images: []securitywatch.Image{{ID: "private-image", Status: "checked", Findings: []securitywatch.Finding{{ID: "CVE-test", Severity: "high"}}}}}}
	if err := securitywatch.Save(path, state); err != nil {
		t.Fatal(err)
	}
	s, err := Export(d, "alpha", "test", "commit", "", path, now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Security.High != 1 || s.Security.Unknown != 1 || s.Security.Status != "attention" || s.Monitoring.Status != "unknown" || s.Projects[0].LastBackup == nil {
		t.Fatalf("lost evidence: %+v", s)
	}
	b, _ := json.Marshal(s)
	for _, secret := range []string{"private.example", "private-user", "/private/", "private/repository", "private-image", "private-error"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("private field leaked")
		}
	}
	out := filepath.Join(dir, "snapshot.json")
	if err := WriteSnapshot(out, s); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(out)
	if info.Mode().Perm() != 0640 {
		t.Fatal("export permissions")
	}
	state.Report.Images = nil
	if err := securitywatch.Save(path, state); err != nil {
		t.Fatal(err)
	}
	s, err = Export(d, "alpha", "test", "commit", "", path, now)
	if err != nil || s.Security.Status != "unknown" || s.Security.Unknown != 1 {
		t.Fatal("incomplete check without known findings must stay unknown")
	}
	state.Report.Images = []securitywatch.Image{{Status: "checked", Findings: []securitywatch.Finding{{Severity: "critical"}}}}
	state.Report.CheckedAt = now.Add(-48 * time.Hour)
	securitywatch.Save(path, state)
	s, err = Export(d, "alpha", "test", "commit", "", path, now)
	if err != nil || s.Security.Status != "unknown" {
		t.Fatal("old report promoted")
	}
}

func TestExportKeepsKnownMonitoringFailureActionable(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := db.Open(filepath.Join(dir, "registry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Now()
	failed := monitor.Target{Name: "failed"}
	missing := monitor.Target{Name: "missing"}
	state := monitor.MonitorState{
		Targets: []monitor.Target{failed, missing},
		Results: map[string]monitor.CheckResult{
			failed.Name: {Target: failed, CheckedAt: now, Healthy: false},
		},
		UpdatedAt: now,
	}
	path := filepath.Join(dir, "monitor.json")
	for _, tc := range []struct {
		name string
		at   time.Time
		want string
	}{
		{"current mixed evidence", now, "attention"},
		{"stale source", now.Add(-10 * time.Minute), "unknown"},
		{"future source", now.Add(2 * time.Minute), "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state.UpdatedAt = tc.at
			data, err := json.Marshal(state)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			s, err := Export(d, "alpha", "test", "commit", path, "", now)
			if err != nil || s.Monitoring.Status != tc.want || s.Monitoring.Unhealthy != 1 || s.Monitoring.Unknown != 1 {
				t.Fatalf("monitoring evidence: %+v, error: %v", s.Monitoring, err)
			}
		})
	}
}
