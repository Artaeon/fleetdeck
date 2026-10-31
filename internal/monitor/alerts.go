package monitor

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// AlertProvider sends notifications when health state changes.
type AlertProvider interface {
	Send(alert Alert) error
	Name() string
}

// Alert represents a notification about a health state change.
type Alert struct {
	Level     string    `json:"level"` // "info", "warning", "critical"
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Target    string    `json:"target"`
	Timestamp time.Time `json:"timestamp"`
}

// targetState tracks consecutive check outcomes for a single target.
type targetState struct {
	ConsecutiveFailures int
	ConsecutiveStatus   int
	PendingStatus       string
	AlertedStatus       string
}

// AlertManager decides when to fire alerts based on state transitions. It only
// alerts when a target crosses the failure threshold (healthy -> unhealthy) or
// recovers (unhealthy -> healthy), not on every individual check.
type AlertManager struct {
	providers        []AlertProvider
	failureThreshold int
	states           map[string]*targetState
	mu               sync.Mutex
	outbox           *alertOutbox
}

// NewAlertManager creates an AlertManager. failureThreshold is the number of
// consecutive failures required before an unhealthy alert is sent.
func NewAlertManager(providers []AlertProvider, failureThreshold int) *AlertManager {
	if failureThreshold < 1 {
		failureThreshold = 1
	}
	return &AlertManager{
		providers:        providers,
		failureThreshold: failureThreshold,
		states:           make(map[string]*targetState),
	}
}

// Process evaluates a check result and sends alerts on state transitions.
func (am *AlertManager) Process(result CheckResult) error {
	am.mu.Lock()
	defer am.mu.Unlock()
	if am.outbox == nil {
		am.process(result)
		return nil
	}
	previous := cloneAlertStates(am.states)
	pending := len(am.outbox.state.Pending)
	am.process(result)
	if err := am.saveOutbox(); err != nil {
		am.states = previous
		am.outbox.state.Pending = am.outbox.state.Pending[:pending]
		return err
	}
	return nil
}

func (am *AlertManager) process(result CheckResult) {

	state, exists := am.states[result.Target.Name]
	if !exists {
		state = &targetState{}
		am.states[result.Target.Name] = state
	}

	status := result.HealthStatus()
	if status == "healthy" {
		wasUnhealthy := state.AlertedStatus != ""
		*state = targetState{}

		if wasUnhealthy {
			am.send(Alert{
				Level:     "info",
				Title:     fmt.Sprintf("%s recovered", result.Target.Name),
				Message:   fmt.Sprintf("%s is healthy again (status %d, %s)", result.Target.Name, result.StatusCode, result.ResponseTime),
				Target:    result.Target.Name,
				Timestamp: result.CheckedAt,
			})
		}
		return
	}

	// Unhealthy check.
	if state.ConsecutiveFailures < am.failureThreshold {
		state.ConsecutiveFailures++
	}
	if state.PendingStatus != status {
		state.PendingStatus = status
		state.ConsecutiveStatus = 0
	}
	if state.ConsecutiveStatus < am.failureThreshold {
		state.ConsecutiveStatus++
	}

	// Mixed failures must still alert. Once an incident is open, require a
	// stable new diagnosis before changing its severity, to avoid flapping.
	if state.ConsecutiveFailures >= am.failureThreshold && state.AlertedStatus != status &&
		(state.AlertedStatus == "" || state.ConsecutiveStatus >= am.failureThreshold) {
		state.AlertedStatus = status
		if status == "blocked" {
			am.send(Alert{
				Level:     "warning",
				Title:     fmt.Sprintf("%s health check blocked", result.Target.Name),
				Message:   "Cloudflare returned a browser challenge. Application health is unknown; this is not a successful check or proof of a server outage.",
				Target:    result.Target.Name,
				Timestamp: result.CheckedAt,
			})
			return
		}

		errMsg := result.Error
		if errMsg != "" {
			// Transport errors may contain a target URL, redirect token or headers.
			// Keep diagnostic detail local; external channels receive a fixed cause.
			errMsg = "health request failed; inspect the private monitor state"
		} else {
			errMsg = fmt.Sprintf("status %d", result.StatusCode)
		}

		am.send(Alert{
			Level:     "critical",
			Title:     fmt.Sprintf("%s is down", result.Target.Name),
			Message:   fmt.Sprintf("%s failed %d consecutive checks: %s", result.Target.Name, state.ConsecutiveFailures, errMsg),
			Target:    result.Target.Name,
			Timestamp: result.CheckedAt,
		})
	}
}

// send dispatches an alert to all configured providers.
func (am *AlertManager) send(alert Alert) {
	if am.outbox != nil {
		if len(am.providers) > 0 {
			delivery := make(map[string]alertDelivery, len(am.providers))
			for _, p := range am.providers {
				delivery[p.Name()] = alertDelivery{}
			}
			am.outbox.state.Pending = append(am.outbox.state.Pending, queuedAlert{Alert: alert, Channels: delivery})
		}
		return
	}
	for _, p := range am.providers {
		// Best-effort delivery; don't let one failing provider block others.
		if err := p.Send(alert); err != nil {
			fmt.Fprintf(os.Stderr, "alert provider %s failed; delivery was not persisted\n", p.Name())
		}
	}
}
