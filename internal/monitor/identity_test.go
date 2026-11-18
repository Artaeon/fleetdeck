package monitor

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestChangedProbeDoesNotInheritAnIncidentAfterRestart(t *testing.T) {
	for _, change := range []struct {
		name string
		edit func(*Target)
	}{
		{"URL", func(v *Target) { v.URL = "https://replacement.example.com/health" }},
		{"method", func(v *Target) { v.Method = "HEAD" }},
		{"expected status", func(v *Target) { v.ExpectedStatus = 204 }},
	} {
		for _, healthy := range []bool{false, true} {
			name := change.name + "/still-down"
			if healthy {
				name = change.name + "/healthy"
			}
			t.Run(name, func(t *testing.T) {
				path := queuePath(t)
				provider := &retryProvider{name: "mail"}
				am, err := NewDurableAlertManager([]AlertProvider{provider}, 1, path)
				if err != nil {
					t.Fatal(err)
				}
				result := makeResult("web", false, 503)
				result.Target.URL = "https://original.example.com/health"
				if err := am.Process(result); err != nil {
					t.Fatal(err)
				}
				if err := am.RetryPending(time.Now()); err != nil {
					t.Fatal(err)
				}
				if err := am.Close(); err != nil {
					t.Fatal(err)
				}
				am, err = NewDurableAlertManager([]AlertProvider{provider}, 1, path)
				if err != nil {
					t.Fatal(err)
				}
				defer am.Close()
				change.edit(&result.Target)
				result.Healthy = healthy
				if err := am.Process(result); err != nil {
					t.Fatal(err)
				}
				if err := am.RetryPending(time.Now()); err != nil {
					t.Fatal(err)
				}
				want := 2
				if healthy {
					want = 1
				}
				if len(provider.alerts) != want {
					t.Fatalf("new probe inherited old incident: got %d alerts, want %d", len(provider.alerts), want)
				}
			})
		}
	}
}

func TestProbeDefaultsDoNotReopenAnIncident(t *testing.T) {
	p := &retryProvider{name: "mail"}
	am := NewAlertManager([]AlertProvider{p}, 1)
	r := makeResult("web", false, 503)
	if err := am.Process(r); err != nil {
		t.Fatal(err)
	}
	r.Target.Method, r.Target.ExpectedStatus = "GET", 200
	r.Target.Interval, r.Target.Timeout = time.Minute, time.Second
	if err := am.Process(r); err != nil {
		t.Fatal(err)
	}
	if len(p.alerts) != 1 {
		t.Fatal("equivalent probe identity reopened incident")
	}
}

func TestLegacyIncidentDoesNotClaimRecoveryAndKeepsPendingWork(t *testing.T) {
	path := queuePath(t)
	p := &retryProvider{name: "mail", fail: true}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := am.Process(makeResult("web", false, 503)); err != nil {
		t.Fatal(err)
	}
	if err := am.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk map[string]any
	if err := json.Unmarshal(data, &disk); err != nil {
		t.Fatal(err)
	}
	delete(disk["states"].(map[string]any)["web"].(map[string]any), "probe_identity")
	data, err = json.Marshal(disk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	am, err = NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	if err := am.Process(makeResult("web", true, 200)); err != nil {
		t.Fatal(err)
	}
	p.fail = false
	if err := am.RetryPending(time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(p.alerts) != 1 || p.alerts[0].Level != "critical" {
		t.Fatal("legacy pending outage lost or false recovery inferred")
	}
}

func TestChangedProbeStartsItsOwnFailureThreshold(t *testing.T) {
	p := &retryProvider{name: "mail"}
	am := NewAlertManager([]AlertProvider{p}, 2)
	r := makeResult("web", false, 503)
	r.Target.URL = "https://original.example.com/health"
	if err := am.Process(r); err != nil {
		t.Fatal(err)
	}
	r.Target.URL = "https://replacement.example.com/health"
	if err := am.Process(r); err != nil {
		t.Fatal(err)
	}
	if len(p.alerts) != 0 {
		t.Fatal("different probes shared their failure threshold")
	}
	if err := am.Process(r); err != nil {
		t.Fatal(err)
	}
	if len(p.alerts) != 1 {
		t.Fatal("new probe did not alert at its own threshold")
	}
}
