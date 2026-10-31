package monitor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
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
	FailureKind  string        `json:"failure_kind,omitempty"`
	Error        string        `json:"error,omitempty"`
	CheckedAt    time.Time     `json:"checked_at"`
}

const FailureCloudflareChallenge = "cloudflare_challenge"

// HealthStatus separates an unobservable application from a failed check.
// Healthy remains false for challenges, including those returning an expected
// status code. Older state files without failure_kind remain readable.
func (r CheckResult) HealthStatus() string {
	if r.FailureKind == FailureCloudflareChallenge {
		return "blocked"
	}
	if r.Healthy {
		return "healthy"
	}
	return "unhealthy"
}

func isCloudflareChallenge(header http.Header) bool {
	return strings.EqualFold(strings.TrimSpace(header.Get("Cf-Mitigated")), "challenge")
}

// Monitor runs periodic health checks against a set of targets and dispatches
// alerts when state changes are detected.
type Monitor struct {
	targets   []Target
	alerts    *AlertManager
	results   map[string]CheckResult
	mu        sync.RWMutex
	saveMu    sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	errors    chan error
	StatePath string
}

// New creates a Monitor that will check the given targets and send alerts
// through the provided providers. failureThreshold is the number of consecutive
// failures before an alert is sent.
func New(targets []Target, providers []AlertProvider, failureThreshold int) *Monitor {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	return &Monitor{
		targets: targets,
		alerts:  NewAlertManager(providers, failureThreshold),
		results: make(map[string]CheckResult),
		done:    make(chan struct{}),
		errors:  make(chan error, 1),
	}
}

// NewWithState creates a Monitor like New but also configures on-disk state
// persistence at the given statePath.
func NewWithState(targets []Target, providers []AlertProvider, failureThreshold int, statePath string) *Monitor {
	m := New(targets, providers, failureThreshold)
	m.StatePath = statePath
	return m
}

// EnableDurableAlerts must be called before Start. The caller closes the queue
// after Stop, so another monitor cannot steal its delivery checkpoint.
func (m *Monitor) EnableDurableAlerts(path string) error {
	am, err := NewDurableAlertManager(m.alerts.providers, m.alerts.failureThreshold, path)
	if err != nil {
		return err
	}
	m.alerts = am
	return nil
}

func (m *Monitor) CloseAlerts() error   { return m.alerts.Close() }
func (m *Monitor) Errors() <-chan error { return m.errors }
func (m *Monitor) fail(err error) {
	select {
	case m.errors <- err:
	default:
	}
	if m.cancel != nil {
		m.cancel()
	}
}

// Start begins the monitoring loop. It runs until Stop is called or the
// context is cancelled.
func (m *Monitor) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)

	go func() {
		defer close(m.done)
		var wg sync.WaitGroup
		if m.alerts.outbox != nil {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					if err := m.alerts.RetryPending(time.Now()); err != nil {
						m.fail(err)
						return
					}
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
					}
				}
			}()
		}

		// Initial check on all targets.
		for _, t := range m.targets {
			if ctx.Err() != nil {
				break
			}
			r := m.CheckOnce(t)
			m.record(r)
		}

		// Per-target tickers tracked with a WaitGroup so Stop() waits for
		// all goroutines to finish.
		for _, t := range m.targets {
			t := t
			interval := t.Interval
			if interval == 0 {
				interval = 30 * time.Second
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.loop(ctx, t, interval)
			}()
		}

		<-ctx.Done()
		wg.Wait()
	}()
}

// Stop cancels the monitoring loop and waits for all goroutines to finish.
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

	client := &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Do not follow away evidence that the original probe was challenged.
		if req.Response != nil && isCloudflareChallenge(req.Response.Header) {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}}

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
	if isCloudflareChallenge(resp.Header) {
		result.FailureKind = FailureCloudflareChallenge
		return result
	}
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

// record stores the check result and notifies the alert manager. If a
// StatePath is configured, it also persists state to disk.
func (m *Monitor) record(r CheckResult) {
	m.mu.Lock()
	m.results[r.Target.Name] = r
	m.mu.Unlock()

	if err := m.alerts.Process(r); err != nil {
		m.fail(fmt.Errorf("cannot persist health alerts: %w", err))
	}

	if m.StatePath != "" {
		if err := m.SaveStateToDisk(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save monitor state: %v\n", err)
		}
	}
}
