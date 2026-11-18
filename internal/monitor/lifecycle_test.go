package monitor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type lifecycleProvider struct {
	fail bool
	sent chan Alert
}

func (*lifecycleProvider) Name() string                { return "fixture" }
func (*lifecycleProvider) DestinationIdentity() string { return destinationHash("fixture") }
func (p *lifecycleProvider) Send(alert Alert) error {
	if p.fail {
		return errors.New("synthetic delivery failure")
	}
	p.sent <- alert
	return nil
}

func TestMonitorLifecycleRetriesPersistedOutageBeforeRecovery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	target := Target{Name: "fixture", URL: server.URL, Timeout: time.Second, Interval: time.Hour}
	p := &lifecycleProvider{fail: true, sent: make(chan Alert, 4)}
	path := queuePath(t)
	am, err := NewDurableAlertManager([]AlertProvider{p}, 1, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := am.Process(CheckResult{Target: target, StatusCode: 503, CheckedAt: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := am.RetryPending(time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := am.Close(); err != nil {
		t.Fatal(err)
	}
	p.fail = false
	m := NewWithState([]Target{target}, []AlertProvider{p}, 1, filepath.Join(t.TempDir(), "observations.json"))
	if err := m.EnableDurableAlerts(path); err != nil {
		t.Fatal(err)
	}
	defer m.CloseAlerts()
	m.Start(context.Background())
	defer m.Stop()
	for _, want := range []string{"critical", "info"} {
		select {
		case got := <-p.sent:
			if got.Level != want {
				t.Fatalf("delivery order: %s, want %s", got.Level, want)
			}
		case err := <-m.Errors():
			t.Fatal(err)
		case <-time.After(5 * time.Second):
			t.Fatal("delivery worker did not recover persisted backlog")
		}
	}
}

func TestMonitorLifecycleSurfacesQueueStorageFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	path := queuePath(t)
	m := New([]Target{{Name: "fixture", URL: server.URL, Timeout: time.Second}}, nil, 1)
	if err := m.EnableDurableAlerts(path); err != nil {
		t.Fatal(err)
	}
	defer m.CloseAlerts()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	m.Start(context.Background())
	defer m.Stop()
	select {
	case err := <-m.Errors():
		if err == nil {
			t.Fatal("missing storage failure")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("monitor silently continued after queue storage failure")
	}
}
