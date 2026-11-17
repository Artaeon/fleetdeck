package securityreport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

func fixture(t *testing.T, now time.Time) (Input, securitywatch.State) {
	t.Helper()
	s := securitywatch.State{Version: 1, Report: securitywatch.Report{CheckedAt: now, Release: securitywatch.Check{Status: "up_to_date"}, Host: securitywatch.Check{Status: "up_to_date"}, Inventory: securitywatch.Check{Status: "checked"}}}
	in := Input{"node-a", filepath.Join(t.TempDir(), "scan.json")}
	if err := securitywatch.Save(in.Path, s); err != nil {
		t.Fatal(err)
	}
	return in, s
}
func TestMissingStaleInvalidAndIncompleteAreNotClean(t *testing.T) {
	now := time.Now()
	in, s := fixture(t, now)
	for _, tc := range []string{"missing", "stale", "future", "empty", "unknown", "public", "invalid", "inventory"} {
		t.Run(tc, func(t *testing.T) {
			copy := s
			switch tc {
			case "stale":
				copy.Report.CheckedAt = now.Add(-27 * time.Hour)
			case "future":
				copy.Report.CheckedAt = now.Add(time.Minute)
			case "empty":
				copy.Report = securitywatch.Report{}
			case "unknown":
				copy.Report.Host.Status = "unknown"
			case "inventory":
				copy.Report.Inventory.Count = 2
			}
			if err := securitywatch.Save(in.Path, copy); err != nil {
				t.Fatal(err)
			}
			switch tc {
			case "missing":
				os.Remove(in.Path)
			case "public":
				os.Chmod(in.Path, 0644)
			case "invalid":
				os.WriteFile(in.Path, []byte("not JSON private error"), 0600)
			}
			r, err := Build([]Input{in}, now, 26*time.Hour)
			if err != nil || r.Nodes[0].Unknown == 0 || r.Level() == "info" {
				t.Fatalf("%s accepted: %+v %v", tc, r, err)
			}
		})
	}
}
func TestReportPreservesKnownRisksAndOmitsPrivateEvidence(t *testing.T) {
	now := time.Now()
	in, s := fixture(t, now)
	s.Report.Host = securitywatch.Check{Status: "unknown", Code: "private-error-token", RebootRequired: true}
	s.Report.Images = []securitywatch.Image{{ID: "private-image-id", Status: "checked", Findings: []securitywatch.Finding{{ID: "private-component", Severity: "critical"}}}}
	s.History = []securitywatch.Observation{{CheckedAt: now.Add(-6 * time.Hour), Critical: 3, Warnings: 4, Unknown: 1}}
	if err := securitywatch.Save(in.Path, s); err != nil {
		t.Fatal(err)
	}
	r, err := Build([]Input{in}, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if r.Nodes[0].Status != "attention" || r.Nodes[0].Unknown != 1 || !r.Nodes[0].RebootRequired || r.Nodes[0].Previous == nil || r.Level() != "critical" {
		t.Fatal(r)
	}
	b, _ := json.Marshal(r)
	for _, out := range []string{string(b), r.Text("de", false), r.Text("en", true)} {
		for _, private := range []string{"private-image-id", "private-error-token", "private-component", in.Path} {
			if strings.Contains(out, private) {
				t.Fatal("leaked private scan evidence")
			}
		}
	}
	if len(r.NotCovered) != 4 {
		t.Fatal("coverage gaps omitted")
	}
}
func TestInputsAndCompactSize(t *testing.T) {
	for _, values := range [][]string{nil, {"node-a=one", "node-a=two"}, {"bad\nlabel=file"}, {"=file"}, {"node-a="}} {
		if _, err := Inputs(values); err == nil {
			t.Fatal(values)
		}
	}
	r := Report{GeneratedAt: time.Now()}
	for i := 0; i < 64; i++ {
		r.Nodes = append(r.Nodes, Node{Label: fmt.Sprintf("node-%02d-", i) + strings.Repeat("x", 50), Critical: 999999, Warnings: 999999, Unknown: 999999, Actions: []string{"plan_restart", "review_runtime_permissions"}})
	}
	for _, language := range []string{"en", "de"} {
		if n := len([]rune(r.Text(language, true))); n > 3800 {
			t.Fatalf("compact message too long: %d", n)
		}
	}
}

func TestCompactReportPrioritizesRiskWithoutChangingEnrollment(t *testing.T) {
	r := Report{GeneratedAt: time.Now()}
	for i := 0; i < 12; i++ {
		r.Nodes = append(r.Nodes, Node{Label: fmt.Sprintf("node-%02d", i), Status: "checked"})
	}
	r.Nodes[10].Critical = 2
	r.Nodes[11].Unknown = 1
	for _, language := range []string{"en", "de"} {
		text := r.Text(language, true)
		if !strings.Contains(text, "node-10: 2") || !strings.Contains(text, "node-11:") || strings.Contains(text, "node-09:") {
			t.Fatalf("important nodes omitted: %s", text)
		}
		if !strings.Contains(text, "12") || !strings.Contains(text, "+ 4") {
			t.Fatal("truncation or full-fleet total omitted")
		}
	}
	for i, n := range r.Nodes {
		if n.Label != fmt.Sprintf("node-%02d", i) {
			t.Fatal("presentation changed notification enrollment order")
		}
	}
}

func TestIncompleteImageCoverageCannotLookClean(t *testing.T) {
	now := time.Now()
	for _, tc := range []string{"missing-image", "duplicate-image", "duplicate-container", "empty-container", "invalid-inventory-status"} {
		t.Run(tc, func(t *testing.T) {
			in, s := fixture(t, now)
			s.Report.Inventory.Count = 1
			s.Report.Containers = []securitywatch.Container{{ID: "container-a", ImageID: "image-a"}}
			s.Report.Images = []securitywatch.Image{{ID: "image-a", Status: "checked"}}
			switch tc {
			case "missing-image":
				s.Report.Images[0].ID = "different-image"
			case "duplicate-image":
				s.Report.Images = append(s.Report.Images, s.Report.Images[0])
			case "duplicate-container":
				s.Report.Containers = append(s.Report.Containers, s.Report.Containers[0])
				s.Report.Inventory.Count = 2
			case "empty-container":
				s.Report.Containers[0].ID = ""
			case "invalid-inventory-status":
				s.Report.Inventory.Status = "update_available"
			}
			if err := securitywatch.Save(in.Path, s); err != nil {
				t.Fatal(err)
			}
			r, err := Build([]Input{in}, now, 26*time.Hour)
			if err != nil || r.Nodes[0].Unknown == 0 || r.Level() == "info" {
				t.Fatalf("incomplete coverage accepted: %+v %v", r, err)
			}
		})
	}
}

func TestStaleObservationDoesNotClearPendingRestart(t *testing.T) {
	now := time.Now()
	in, s := fixture(t, now.Add(-27*time.Hour))
	s.Report.Host.RebootRequired = true
	if err := securitywatch.Save(in.Path, s); err != nil {
		t.Fatal(err)
	}
	r, err := Build([]Input{in}, now, 26*time.Hour)
	if err != nil || !r.Nodes[0].RebootRequired || r.Nodes[0].Status != "unknown" || !strings.Contains(r.Text("en", true), "previously reported pending restart") {
		t.Fatalf("stale restart silently cleared: %+v %v", r, err)
	}
}

type testProvider struct {
	name string
	fail bool
	sent int
}

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) Send(monitor.Alert) error {
	p.sent++
	if p.fail {
		return fmt.Errorf("private transport error")
	}
	return nil
}
func TestDeliveryRetriesOnlyFailedChannelsAndRejectsChangedEnrollment(t *testing.T) {
	now := time.Now()
	in, _ := fixture(t, now)
	r, err := Build([]Input{in}, now, 26*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	path := filepath.Join(dir, "delivery.json")
	a, b := &testProvider{name: "telegram"}, &testProvider{name: "email", fail: true}
	providers := []monitor.AlertProvider{a, b}
	if err := Deliver(path, r, "en", 7*24*time.Hour, providers); err == nil {
		t.Fatal("delivery error hidden")
	}
	b.fail = false
	if err := Deliver(path, r, "en", 7*24*time.Hour, providers); err != nil {
		t.Fatal(err)
	}
	if a.sent != 1 || b.sent != 2 {
		t.Fatal("successful channel duplicated")
	}
	r.GeneratedAt = now.Add(8 * 24 * time.Hour)
	if err := Deliver(path, r, "en", 7*24*time.Hour, providers); err != nil {
		t.Fatal(err)
	}
	if a.sent != 2 || b.sent != 3 {
		t.Fatal("next period not delivered")
	}
	r.Nodes[0].Label = "node-b"
	if err := Deliver(path, r, "en", 7*24*time.Hour, providers); err == nil {
		t.Fatal("silent enrollment change")
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "private transport") {
		t.Fatal("leaked provider error")
	}
	i, _ := os.Stat(path)
	if i.Mode().Perm() != 0600 {
		t.Fatal(i.Mode())
	}
}
func TestDeliveryRejectsPublicAndScannerState(t *testing.T) {
	now := time.Now()
	in, _ := fixture(t, now)
	r, _ := Build([]Input{in}, now, 26*time.Hour)
	providers := []monitor.AlertProvider{&testProvider{name: "telegram"}}
	if err := Deliver(in.Path, r, "en", 7*24*time.Hour, providers); err == nil {
		t.Fatal("overwrote scanner state")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0755)
	if err := Deliver(filepath.Join(dir, "delivery.json"), r, "en", 7*24*time.Hour, providers); err == nil {
		t.Fatal("public state directory accepted")
	}
}
