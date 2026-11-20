package fleetexport

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

func TestReportSnapshotExcludesInventoryAndPreservesScanAge(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "state.json")
	now := time.Now().UTC()
	if _, err := ReportSnapshot("node-a", p, now); err == nil {
		t.Fatal("missing scan appeared healthy")
	}
	scan := now.Add(-time.Hour)
	state := securitywatch.State{Version: 1, Report: securitywatch.Report{CheckedAt: scan, Host: securitywatch.Check{Status: "up_to_date", Code: "private-host-detail"}, Release: securitywatch.Check{Status: "up_to_date"}, Inventory: securitywatch.Check{Status: "checked"}, Images: []securitywatch.Image{{ID: "private-image", Status: "checked", Findings: []securitywatch.Finding{{Severity: "HIGH", ID: "private-finding"}}}}}}
	if err := securitywatch.Save(p, state); err != nil {
		t.Fatal(err)
	}
	s, err := ReportSnapshot("node-a", p, now)
	if err != nil || s.Security.High != 1 || s.Security.CheckedAt == nil || !s.Security.CheckedAt.Equal(scan) || len(s.Projects) != 0 || s.Version != "" || s.Commit != "" {
		t.Fatal(s, err)
	}
	path := filepath.Join(dir, "source.json")
	if err := WriteSnapshot(path, s); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || bytes.Contains(b, []byte("private-")) {
		t.Fatal("projection leaked private data", err)
	}
	if _, err := fleet.Decode(bytes.NewReader(b), "node-a", now); err != nil {
		t.Fatal(err)
	}
	if _, err := ReportSnapshot("node-a", p, scan.Add(-time.Minute)); err == nil {
		t.Fatal("accepted future scan")
	}
	s, err = ReportSnapshot("node-a", p, now.Add(48*time.Hour))
	if err != nil || s.Security.Status != "unknown" || !s.Security.CheckedAt.Equal(scan) {
		t.Fatal("stale scan lost its time", s, err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if len(raw) != 8 {
		t.Fatalf("unexpected projection fields %v", raw)
	}
}
