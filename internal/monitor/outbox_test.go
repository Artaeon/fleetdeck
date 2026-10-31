package monitor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type retryProvider struct {
	name   string
	fail   bool
	alerts []Alert
}

func (p *retryProvider) Name() string { return p.name }
func (p *retryProvider) Send(a Alert) error {
	p.alerts = append(p.alerts, a)
	if p.fail {
		return errors.New("secret-token-in-provider-error")
	}
	return nil
}
func queuePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "private", "state.json")
}

func TestDurableAlertsRestartPartialDeliveryAndRecoveryOrder(t *testing.T) {
	path := queuePath(t)
	mail, telegram := &retryProvider{name: "mail"}, &retryProvider{name: "telegram", fail: true}
	am, err := NewDurableAlertManager([]AlertProvider{mail, telegram}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err = am.Process(makeResult("web", false, 503)); err != nil {
		t.Fatal(err)
	}
	if len(mail.alerts) != 0 {
		t.Fatal("check synchronously sent an alert")
	}
	if err = am.RetryPending(now); err != nil {
		t.Fatal(err)
	}
	if err = am.Process(makeResult("web", true, 200)); err != nil {
		t.Fatal(err)
	}
	if err = am.RetryPending(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(mail.alerts) != 2 || len(telegram.alerts) != 1 {
		t.Fatal("one failed channel blocked another or ignored backoff")
	}
	bytes, _ := os.ReadFile(path)
	if strings.Contains(string(bytes), "secret-token") {
		t.Fatal("provider error leaked")
	}
	am.Close()
	am, err = NewDurableAlertManager([]AlertProvider{mail, telegram}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	telegram.fail = false
	if err = am.RetryPending(now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = am.RetryPending(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(mail.alerts) != 2 || len(telegram.alerts) != 3 {
		t.Fatal("restart lost pending work or repeated acknowledged delivery")
	}
	if telegram.alerts[1].Level != "critical" || telegram.alerts[2].Level != "info" {
		t.Fatal("recovery overtook outage")
	}
	status, err := ReadDeliveryStatus(path)
	if err != nil || status.Pending != 0 {
		t.Fatalf("backlog: %+v %v", status, err)
	}
	if err = am.Process(makeResult("web", true, 200)); err != nil {
		t.Fatal(err)
	}
	if len(am.outbox.state.Pending) != 0 {
		t.Fatal("duplicate recovery after restart")
	}
}

func TestDurableAlertsPersistOpenIncidentWithoutRepeatingIt(t *testing.T) {
	path := queuePath(t)
	p := &retryProvider{name: "mail"}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 2, path)
	if err != nil {
		t.Fatal(err)
	}
	am.Process(makeResult("web", false, 503))
	am.Close()
	am, err = NewDurableAlertManager([]AlertProvider{p}, 2, path)
	if err != nil {
		t.Fatal(err)
	}
	am.Process(makeResult("web", false, 503))
	am.RetryPending(time.Now())
	am.Close()
	am, err = NewDurableAlertManager([]AlertProvider{p}, 2, path)
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	am.Process(makeResult("web", false, 503))
	am.RetryPending(time.Now())
	if len(p.alerts) != 1 {
		t.Fatal("threshold or acknowledged incident not preserved")
	}
}

func TestDurableAlertsRefuseUnsafeOrCorruptStorage(t *testing.T) {
	for _, kind := range []string{"symlink", "public", "corrupt", "future-version"} {
		t.Run(kind, func(t *testing.T) {
			path := queuePath(t)
			os.MkdirAll(filepath.Dir(path), 0700)
			switch kind {
			case "symlink":
				os.Symlink(filepath.Join(t.TempDir(), "outside"), path)
			case "public":
				os.WriteFile(path, []byte(`{}`), 0644)
			case "corrupt":
				os.WriteFile(path, []byte(`{"pending":`), 0600)
			case "future-version":
				os.WriteFile(path, []byte(`{"version":2,"states":{}}`), 0600)
			}
			if am, err := NewDurableAlertManager(nil, 1, path); err == nil {
				am.Close()
				t.Fatal("unsafe state was silently replaced")
			}
		})
	}
}

func TestDurableAlertsSingleOwnerAndRemovedProvider(t *testing.T) {
	path := queuePath(t)
	p := &retryProvider{name: "mail"}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := NewDurableAlertManager([]AlertProvider{p}, 1, path); err == nil {
		other.Close()
		t.Fatal("second writer accepted")
	}
	am.Process(makeResult("web", false, 503))
	am.Close()
	am, err = NewDurableAlertManager(nil, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	am.RetryPending(time.Now())
	status, err := ReadDeliveryStatus(path)
	if err != nil || status.Channels["mail"].Pending != 1 {
		t.Fatal("removed channel silently discarded")
	}
}

func TestDurableAlertsStorageFailureDoesNotAcknowledgeTransition(t *testing.T) {
	path := queuePath(t)
	p := &retryProvider{name: "mail"}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	// Deterministic failed rename, including when tests run as root.
	os.Remove(path)
	os.Mkdir(path, 0700)
	if err := am.Process(makeResult("web", false, 503)); err == nil {
		t.Fatal("unpersisted transition accepted")
	}
	if len(am.states) != 0 || len(p.alerts) != 0 {
		t.Fatal("failed storage advanced incident")
	}
	os.Remove(path)
	if err := am.Process(makeResult("web", false, 503)); err != nil {
		t.Fatal(err)
	}
	am.RetryPending(time.Now())
	if len(p.alerts) != 1 {
		t.Fatal("retry lost alert")
	}
}

func TestDurableAlertsQueueLimitRetainsExistingWork(t *testing.T) {
	path := queuePath(t)
	p := &retryProvider{name: "mail"}
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	for i := 0; i < maxQueuedAlerts; i++ {
		am.outbox.state.Pending = append(am.outbox.state.Pending, queuedAlert{Alert: Alert{Target: "existing"}, Channels: map[string]alertDelivery{"mail": {}}})
	}
	if err := am.saveOutbox(); err != nil {
		t.Fatal(err)
	}
	if err := am.Process(makeResult("new", false, 503)); err == nil {
		t.Fatal("overflow silently accepted")
	}
	status, err := ReadDeliveryStatus(path)
	if err != nil || status.Pending != maxQueuedAlerts || am.states["new"] != nil {
		t.Fatal("overflow discarded or acknowledged work")
	}
}
