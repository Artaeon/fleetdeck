package cmd

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

type reportingTestChannel struct {
	alerts      []monitor.Alert
	unavailable bool
}

func (c *reportingTestChannel) Name() string                { return "telegram" }
func (c *reportingTestChannel) DestinationIdentity() string { return strings.Repeat("a", 64) }
func (c *reportingTestChannel) Send(alert monitor.Alert) error {
	if c.unavailable {
		return errors.New("test transport unavailable")
	}
	c.alerts = append(c.alerts, alert)
	return nil
}

func TestIndependentCheckAndDurableTelegramIncident(t *testing.T) {
	now := time.Now().UTC()
	projection := func(at time.Time, pending int) string {
		b, err := json.Marshal(fleet.ReportRuns{Daily: fleet.ReportRun{LastEvaluation: at}, Weekly: fleet.ReportRun{LastEvaluation: now, Pending: pending}})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	input := projection(now, 0)
	for _, tc := range []struct{ input, want string }{{input, "current"}, {projection(now, 1), "pending"}, {projection(now.Add(-3*time.Hour), 0), "stale"}, {"", "unavailable"}, {"{}", "unavailable"}} {
		if got := deliveryCondition(strings.NewReader(tc.input), now, 2*time.Hour); got != tc.want {
			t.Fatalf("got %s want %s", got, tc.want)
		}
	}
	channel := &reportingTestChannel{}
	state := filepath.Join(t.TempDir(), "private", "delivery.json")
	step := func(body string) {
		manager, err := monitor.NewDurableAlertManager([]monitor.AlertProvider{channel}, 2, state)
		if err != nil {
			t.Fatal(err)
		}
		defer manager.Close()
		status := deliveryCondition(strings.NewReader(body), now, 2*time.Hour)
		result := monitor.CheckResult{Target: monitor.Target{Name: "node-a", URL: "fleetdeck-report://node-a"}, Healthy: status == "current", CheckedAt: now, Notification: deliveryNotice(status, "de")}
		if err = manager.Process(result); err != nil {
			t.Fatal(err)
		}
		err = manager.RetryPending(now)
		if err != nil && !channel.unavailable {
			t.Fatal(err)
		}
		now = now.Add(5 * time.Minute)
	}
	step(input)
	step("")
	if len(channel.alerts) != 0 {
		t.Fatal("early notification")
	}
	channel.unavailable = true
	step("")
	q, err := monitor.ReadDeliveryStatus(state)
	if err != nil || q.Pending != 1 {
		t.Fatal(q, err)
	}
	channel.unavailable = false
	step("")
	step("")
	if len(channel.alerts) != 1 || channel.alerts[0].Title != "Berichtsversand prüfen" {
		t.Fatal("outage or dedup", channel.alerts)
	}
	step(input)
	step(input)
	if len(channel.alerts) != 2 || channel.alerts[1].Level != "info" || channel.alerts[0].Incident == "" || channel.alerts[1].Incident != channel.alerts[0].Incident {
		t.Fatal("recovery", channel.alerts)
	}
	for _, reasons := range []struct{ input, word string }{{projection(now, 1), "Zustellung"}, {projection(now.Add(-4*time.Hour), 0), "überfällig"}} {
		step(reasons.input)
		step(reasons.input)
		if !strings.Contains(channel.alerts[len(channel.alerts)-1].Message, reasons.word) {
			t.Fatal("wrong report alert", reasons.word)
		}
		step(projection(now, 0))
	}
}
