package monitor

import (
	"testing"
	"time"
)

type incidentCapture struct{ alerts []Alert }

func (p *incidentCapture) Name() string       { return "test" }
func (p *incidentCapture) Send(a Alert) error { p.alerts = append(p.alerts, a); return nil }

func TestOneReferencePerOutage(t *testing.T) {
	p := &incidentCapture{}
	m := NewAlertManager([]AlertProvider{p}, 1)
	now := time.Now()
	target := Target{Name: "service-a", URL: "https://example.org"}
	for i, code := range []int{503, 503, 200, 503} {
		result := CheckResult{Target: target, CheckedAt: now.Add(time.Duration(i) * time.Minute), StatusCode: code, Healthy: code == 200}
		if err := m.Process(result); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.alerts) != 3 || p.alerts[0].Incident == "" || p.alerts[1].Incident != p.alerts[0].Incident || p.alerts[2].Incident == p.alerts[0].Incident {
		t.Fatalf("incorrect incident correlation: %+v", p.alerts)
	}
}
