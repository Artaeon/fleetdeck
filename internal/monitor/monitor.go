package monitor

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Target describes an endpoint to monitor.
type Target struct {
	Name           string        `json:"name"`
	URL            string        `json:"url"`
	Method         string        `json:"method"`
	ExpectedStatus int           `json:"expected_status"`
	Timeout        time.Duration `json:"timeout"`
	Interval       time.Duration `json:"interval"`
}

// CheckResult holds the outcome of a single health check.
type CheckResult struct {
	Target       Target        `json:"target"`
	StatusCode   int           `json:"status_code"`
	ResponseTime time.Duration `json:"response_time"`
	Healthy      bool          `json:"healthy"`
	Error        string        `json:"error,omitempty"`
	CheckedAt    time.Time     `json:"checked_at"`
}

// Monitor runs periodic health checks against a set of targets and dispatches
// alerts when state changes are detected.
type Monitor struct {
	targets  []Target
	alerts   *AlertManager
	results  map[string]CheckResult
	mu       sync.RWMutex
	cancel   context.CancelFunc
	done     chan struct{}
}

// New creates a Monitor that will check the given targets and send alerts
// through the provided providers.
func New(targets []Target, providers []AlertProvider) *Monitor {
	return &Monitor{
		targets: targets,
		alerts:  NewAlertManager(providers, 3),
		results: make(map[string]CheckResult),
		done:    make(chan struct{}),
	}
}

// Start begins the monitoring loop. It runs until Stop is called or the
// context is cancelled.
func (m *Monitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	go func() {
		defer close(m.done)

		// Initial check on all targets.
		for _, t := range m.targets {
			r := m.CheckOnce(t)
			m.record(r)
		}

		// Per-target tickers.
		for _, t := range m.targets {
			t := t
			interval := t.Interval
			if interval == 0 {
				interval = 30 * time.Second
			}
			go m.loop(ctx, t, interval)
		}

		<-ctx.Done()
	}()
}

// Stop cancels the monitoring loop and waits for it to finish.
func (m *Monitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	<-m.done
}

// Status returns the latest check result for every target.
func (m *Monitor) Status() []CheckResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make([]CheckResult, 0, len(m.results))
	for _, r := range m.results {
		results = append(results, r)
	}
	return results
}

// CheckOnce performs a single health check against the target.
func (m *Monitor) CheckOnce(target Target) CheckResult {
	result := CheckResult{
		Target:    target,
		CheckedAt: time.Now(),
	}

	method := target.Method
	if method == "" {
		method = http.MethodGet
	}
	expectedStatus := target.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}
	timeout := target.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequest(method, target.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	start := time.Now()
	resp, err := client.Do(req)
	result.ResponseTime = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Healthy = resp.StatusCode == expectedStatus

	return result
}

// loop runs periodic checks for a single target.
func (m *Monitor) loop(ctx context.Context, target Target, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r := m.CheckOnce(target)
			m.record(r)
		}
	}
}

// record stores the check result and notifies the alert manager.
func (m *Monitor) record(r CheckResult) {
	m.mu.Lock()
	m.results[r.Target.Name] = r
	m.mu.Unlock()

	m.alerts.Process(r)
}
