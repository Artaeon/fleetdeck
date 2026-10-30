package securitywatch

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRunner struct {
	scanFails bool
	aptOutput *string
	scanID    string
	scanJSON  []byte
	calls     [][]string
}

var imageID = "sha256:" + strings.Repeat("a", 64)

func (r *fakeRunner) Run(ctx context.Context, program string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{program}, args...))
	if program == "apt-get" {
		if r.aptOutput != nil {
			return []byte(*r.aptOutput), nil
		}
		return []byte("Inst openssl [old] (new distribution-security)\n"), nil
	}
	if program == "docker" && args[0] == "ps" {
		return []byte(strings.Repeat("b", 12)), nil
	}
	if program == "docker" && args[0] == "inspect" {
		if strings.Contains(args[2], ".Env") {
			return nil, fmt.Errorf("requested secret environment")
		}
		return []byte(fmt.Sprintf(`{"id":%q,"image":%q,"user":"0","privileged":true,"network":"bridge","security":[],"caps":[],"mounts":[{"Source":"/var/run/docker.sock","Destination":"/run/docker.sock"}]}`, strings.Repeat("b", 64), imageID)), nil
	}
	if program == "trivy" {
		if r.scanFails {
			return nil, fmt.Errorf("unavailable fixture")
		}
		if r.scanJSON != nil {
			return r.scanJSON, nil
		}
		id := r.scanID
		if id == "" {
			id = imageID
		}
		return []byte(fmt.Sprintf(`{"SchemaVersion":2,"ArtifactType":"container_image","Metadata":{"ImageID":%q},"Results":[{"Vulnerabilities":[{"VulnerabilityID":"CVE-2026-12345","Severity":"CRITICAL"},{"VulnerabilityID":"CVE-2026-12345","Severity":"CRITICAL"}]}]}`, id)), nil
	}
	return nil, fmt.Errorf("unexpected program")
}
func testChecker(t *testing.T, installed string) (*Checker, *fakeRunner) {
	t.Helper()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/releases/latest") {
			t.Error(r.URL.Path)
		}
		fmt.Fprint(w, `{"tag_name":"v1.2.3","draft":false,"prerelease":false}`)
	}))
	t.Cleanup(api.Close)
	checker := New()
	runner := &fakeRunner{}
	checker.Runner = runner
	checker.HTTP = api.Client()
	checker.API = api.URL
	checker.HostOS = "linux"
	checker.IndexStamp = filepath.Join(t.TempDir(), "stamp")
	checker.RebootMarker = filepath.Join(t.TempDir(), "reboot-required")
	os.WriteFile(checker.IndexStamp, nil, 0600)
	return checker, runner
}
func TestChecksUseImmutableImagesAndKeepUnknownFailures(t *testing.T) {
	c, r := testChecker(t, "v1.0.0")
	report := c.Run(context.Background(), "example/fleetdeck", "v1.0.0")
	if report.Release.Status != "update_available" || report.Host.Status != "update_available" || report.Images[0].Status != "checked" || len(report.Images[0].Findings) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	critical, _, unknown := report.Counts()
	if critical != 3 || unknown != 0 {
		t.Fatal(critical, unknown)
	}
	last := r.calls[len(r.calls)-1]
	if last[len(last)-1] != imageID || !strings.Contains(strings.Join(last, " "), "--image-src docker") {
		t.Fatal("scanned mutable reference", last)
	}
	r.scanFails = true
	report = c.Run(context.Background(), "example/fleetdeck", "dev")
	_, _, unknown = report.Counts()
	if unknown < 2 || report.Images[0].Status != "unknown" {
		t.Fatal("failed checks became green")
	}
}
func TestReleaseComparisonAndInvalidMetadata(t *testing.T) {
	c, _ := testChecker(t, "")
	for _, tc := range []struct{ installed, status string }{{"v1.2.3", "up_to_date"}, {"1.2.2", "update_available"}, {"v1.2.3-rc.1", "update_available"}, {"v1.3.0", "up_to_date"}, {"dev", "unknown"}, {"1.2", "unknown"}} {
		if result := c.release(context.Background(), "example/fleetdeck", tc.installed); result.Status != tc.status {
			t.Fatal(tc, result)
		}
	}
	if result := c.release(context.Background(), "https://invalid/target", "v1.2.3"); result.Status != "unknown" {
		t.Fatal(result)
	}
}
func TestMismatchedScanAndStaleIndexesAreUnknown(t *testing.T) {
	c, r := testChecker(t, "")
	r.scanID = "sha256:" + strings.Repeat("c", 64)
	if result := c.scan(context.Background(), imageID); result.Status != "unknown" {
		t.Fatal(result)
	}
	old := time.Now().Add(-72 * time.Hour)
	os.Chtimes(c.IndexStamp, old, old)
	result := c.host(context.Background())
	if result.Status != "unknown" || result.Count != 1 {
		t.Fatal(result)
	}
	r.scanJSON = []byte(`{}`)
	if result := c.scan(context.Background(), imageID); result.Status != "unknown" {
		t.Fatal("empty scanner result accepted")
	}
}

func TestImageScanUsesConfiguredBudgetAndSequentialLayers(t *testing.T) {
	c, r := testChecker(t, "")
	c.ScanTimeout = 15 * time.Minute
	if result := c.scan(context.Background(), imageID); result.Status != "checked" {
		t.Fatal(result)
	}
	args := strings.Join(r.calls[len(r.calls)-1], " ")
	if !strings.Contains(args, "--timeout 15m0s") || !strings.Contains(args, "--parallel 1") {
		t.Fatal(args)
	}
}

type provider struct {
	name string
	fail bool
	sent []monitor.Alert
}

func (p *provider) Name() string { return p.name }
func (p *provider) Send(alert monitor.Alert) error {
	p.sent = append(p.sent, alert)
	if p.fail {
		return fmt.Errorf("fixture delivery failure")
	}
	return nil
}
func TestNotificationsRetryOnlyFailedChannelsAndNeverRecoverOnUnknown(t *testing.T) {
	state := State{Version: 1}
	a := &provider{name: "telegram"}
	b := &provider{name: "email", fail: true}
	providers := []monitor.AlertProvider{a, b}
	report := Report{CheckedAt: time.Now(), Release: Check{Status: "update_available"}, Host: Check{Status: "up_to_date"}, Inventory: Check{Status: "checked"}}
	if err := Notify(&state, report, "test-server", providers); err == nil {
		t.Fatal("delivery failure hidden")
	}
	b.fail = false
	report.CheckedAt = report.CheckedAt.Add(time.Hour)
	if err := Notify(&state, report, "test-server", providers); err != nil {
		t.Fatal(err)
	}
	if len(a.sent) != 1 || len(b.sent) != 2 {
		t.Fatal("successful channel repeated")
	}
	report.Release.Status = "unknown"
	if err := Notify(&state, report, "test-server", providers); err != nil {
		t.Fatal(err)
	}
	if a.sent[len(a.sent)-1].Level == "info" {
		t.Fatal("unknown check produced recovery")
	}
	report.Release.Status = "up_to_date"
	if err := Notify(&state, report, "test-server", providers); err != nil {
		t.Fatal(err)
	}
	if a.sent[len(a.sent)-1].Level != "info" {
		t.Fatal("missing verified recovery")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Save(path, state); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	loaded, err := Load(path)
	if err != nil || len(loaded.Deliveries) != 2 {
		t.Fatal(loaded, err)
	}
	data, _ := json.Marshal(state)
	if strings.Contains(string(data), "fixture delivery failure") {
		t.Fatal("stored private provider errors")
	}
	os.Chmod(path, 0644)
	if _, err = Load(path); err == nil {
		t.Fatal("read public state")
	}
}

func TestPendingRebootRemainsVisibleAfterPackagesAreCurrent(t *testing.T) {
	c, runner := testChecker(t, "")
	empty := "0 upgraded, 0 newly installed, 0 to remove\n"
	runner.aptOutput = &empty
	if got := c.host(context.Background()); got.Status != "up_to_date" || got.RebootRequired {
		t.Fatal(got)
	}
	if err := os.WriteFile(c.RebootMarker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	got := c.host(context.Background())
	if got.Status != "up_to_date" || !got.RebootRequired || got.Count != 0 {
		t.Fatal(got)
	}
	report := Report{Host: got}
	_, warnings, unknown := report.Counts()
	if warnings != 1 || unknown != 0 {
		t.Fatal(warnings, unknown)
	}
	p := &provider{name: "test"}
	state := State{}
	if err := Notify(&state, report, "test-server", []monitor.AlertProvider{p}); err != nil {
		t.Fatal(err)
	}
	if len(p.sent) != 1 || p.sent[0].Level != "warning" || !strings.Contains(p.sent[0].Message, "restart") {
		t.Fatal(p.sent)
	}
	// Stale indexes remain unknown without losing the independently known reboot.
	old := time.Now().Add(-72 * time.Hour)
	os.Chtimes(c.IndexStamp, old, old)
	got = c.host(context.Background())
	if got.Status != "unknown" || !got.RebootRequired {
		t.Fatal(got)
	}
	os.Remove(c.RebootMarker)
	if err := os.Symlink(c.IndexStamp, c.RebootMarker); err != nil {
		t.Fatal(err)
	}
	got = c.host(context.Background())
	if got.Status != "unknown" || got.Code != "host_reboot_status_unverified" {
		t.Fatal(got)
	}
}
