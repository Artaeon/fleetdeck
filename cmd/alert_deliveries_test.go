package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

func TestAlertDeliveriesReadsAllQueuesWithoutInitialization(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	write := func(name, data string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	binding := strings.Repeat("a", 64)
	relay := write("relay.json", fmt.Sprintf(`{"version":2,"since":"initial","pending":{"cursor":"private-cursor","alert":{"target":"private-host","message":"private-event","timestamp":"2025-01-01T00:00:01Z"},"destinations":{"telegram":"%s","email":"%s"},"acknowledged":{"email":true}}}`, binding, binding))
	report := write("report.json", fmt.Sprintf(`{"version":2,"labels":"private-fleet","sent":{},"pending":{"email":{"destination":"%s","queued_at":"2025-01-01T01:00:00Z","alert":{"message":"private-report","target":"fleet","title":"FleetDeck security report","timestamp":"2025-01-01T00:00:00Z"}}}}`, binding))
	health := write("health.json", `{"version":1,"states":{},"pending":[{"alert":{"target":"private-site","level":"critical","timestamp":"2024-12-31T23:00:00Z"},"channels":{"telegram":{"delivered":false}}}]}`)
	var out bytes.Buffer
	c := newAlertDeliveriesCmd()
	c.SetOut(&out)
	c.SetArgs([]string{"--health-state", health, "--relay-state", relay, "--report-state", report})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "private-") || strings.Contains(out.String(), binding) {
		t.Fatal("private content in status")
	}
	var views map[string]monitor.Backlog
	if err := json.Unmarshal(out.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, kind, channel, since string }{{"health", "health_failure", "telegram", ""}, {"host_activity", "host_activity", "telegram", "2025-01-01T00:00:01Z"}, {"security_report", "security_report", "email", "2025-01-01T01:00:00Z"}} {
		view := views[tc.source]
		if view.Coverage != "tracked" || len(view.Items) != 1 {
			t.Fatal(view)
		}
		item := view.Items[0]
		if item.Kind != tc.kind || item.Reference == "" || item.Channel != tc.channel || item.ObservedAt == nil {
			t.Fatal(item)
		}
		if tc.since == "" {
			if item.PendingSince != nil {
				t.Fatal("invented queue time")
			}
		} else if item.PendingSince == nil || item.PendingSince.Format(time.RFC3339) != tc.since {
			t.Fatal(item)
		}
	}
	out.Reset()
	c.SetArgs([]string{"--report-state", filepath.Join(dir, "missing")})
	c.SilenceUsage = true
	c.SilenceErrors = true
	if err := c.Execute(); err == nil || out.Len() != 0 {
		t.Fatal("unreadable queue presented as complete")
	}
}

func TestDeliveryOverviewMarksLegacyCoverage(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0700)
	relay, report := filepath.Join(dir, "relay"), filepath.Join(dir, "report")
	if err := os.WriteFile(relay, []byte(`{"since":"start","pending":{"cursor":"next","delivered":{"email":true}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(report, []byte(`{"version":1,"labels":"test","sent":{}}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := newAlertDeliveriesCmd()
	var out bytes.Buffer
	c.SetOut(&out)
	c.SetArgs([]string{"--relay-state", relay, "--report-state", report})
	if err := c.Execute(); err != nil {
		t.Fatal(err)
	}
	var views map[string]monitor.Backlog
	if err := json.Unmarshal(out.Bytes(), &views); err != nil {
		t.Fatal(err)
	}
	if views["host_activity"].Coverage != "legacy_unknown_recipients" || len(views["host_activity"].Items) != 1 || views["host_activity"].Items[0].Channel != "" || views["security_report"].Coverage != "legacy_successes_only" {
		t.Fatal(views)
	}
}
