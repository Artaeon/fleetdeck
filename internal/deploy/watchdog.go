package deploy

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// WatchdogConfig describes how to observe a just-deployed endpoint and
// decide whether the deploy "stuck" — i.e. stayed healthy long enough
// that we can stop worrying about it.
//
// The watchdog is deliberately simple: poll an HTTP URL every Interval,
// counting consecutive failures. Cross FailureThreshold and the deploy
// is declared bad; survive Duration with no threshold breach and it's
// declared good. This is enough to catch the common post-deploy failure
// modes (service starts, health checks pass at the instant of the
// cutover, then OOM-kills a few minutes later) without the complexity
// of metrics-based rollout analysis.
type WatchdogConfig struct {
	// URL is the endpoint to poll. Typically https://<domain> of the
	// project that was just deployed.
	URL string

	// Duration is the total observation window. After this elapses with
	// no threshold breach, the deploy is considered healthy.
	Duration time.Duration

	// Interval is how often to probe. A few seconds is reasonable; much
	// shorter just wastes remote bandwidth.
	Interval time.Duration

	// Timeout is the per-request HTTP timeout. Should be shorter than
	// Interval so a single slow response doesn't block the next probe.
	Timeout time.Duration

	// FailureThreshold is the number of consecutive non-2xx/5xx probes
	// that trigger an unhealthy verdict. 1 would be too jumpy; 3-5 is a
	// good default so a single dropped request doesn't roll back.
	FailureThreshold int

	// ExpectedStatus lets callers match a custom status (e.g. 204 for a
	// health endpoint that returns no body). Zero defaults to 200.
	ExpectedStatus int
}

// WatchResult is the verdict returned when Observe finishes.
type WatchResult struct {
	// Healthy is true if the observation window completed without the
	// failure threshold being hit.
	Healthy bool

	// Probes is how many HTTP requests were issued. Useful in logs so
	// the operator can sanity-check that the watchdog actually ran.
	Probes int

	// ConsecutiveFailures is the peak consecutive-failure count
	// observed. On an unhealthy return this equals FailureThreshold.
	ConsecutiveFailures int

	// LastStatus is the HTTP status code of the final probe (or 0 if
	// the last probe errored out before a response was received).
	LastStatus int

	// LastError is the error from the final probe, if any. Set on
	// unhealthy returns to help the operator diagnose the failure.
	LastError string
}

// Observe runs the watchdog loop against cfg until either the deploy is
// declared unhealthy (returns immediately) or Duration elapses (returns
// healthy). The provided context cancels the loop early — useful for
// wiring into the deploy command's overall timeout.
//
// The function intentionally uses a plain http.Client rather than
// anything fancier so it can be shared between the daemon monitor and
// one-shot post-deploy checks without pulling in dependencies.
func Observe(ctx context.Context, cfg WatchdogConfig) WatchResult {
	cfg = applyWatchdogDefaults(cfg)

	client := &http.Client{Timeout: cfg.Timeout}
	deadline := time.Now().Add(cfg.Duration)

	var result WatchResult
	consecutive := 0

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	// Run an immediate probe so we don't wait a whole Interval before
	// noticing an already-broken deploy.
	probe := func() {
		result.Probes++
		status, err := singleProbe(ctx, client, cfg.URL)
		result.LastStatus = status
		if err != nil {
			result.LastError = err.Error()
		} else {
			result.LastError = ""
		}
		if status == cfg.ExpectedStatus {
			consecutive = 0
			return
		}
		consecutive++
		if consecutive > result.ConsecutiveFailures {
			result.ConsecutiveFailures = consecutive
		}
	}

	probe()
	if consecutive >= cfg.FailureThreshold {
		return result
	}

	for {
		select {
		case <-ctx.Done():
			// Context cancellation is not "unhealthy" — the caller decided
			// to stop observing. Return whatever consecutive state we saw.
			result.Healthy = consecutive < cfg.FailureThreshold
			return result
		case <-ticker.C:
			if time.Now().After(deadline) {
				result.Healthy = true
				return result
			}
			probe()
			if consecutive >= cfg.FailureThreshold {
				return result
			}
		}
	}
}

func applyWatchdogDefaults(cfg WatchdogConfig) WatchdogConfig {
	if cfg.Duration <= 0 {
		cfg.Duration = 5 * time.Minute
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 10 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.ExpectedStatus == 0 {
		cfg.ExpectedStatus = http.StatusOK
	}
	return cfg
}

func singleProbe(ctx context.Context, client *http.Client, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
