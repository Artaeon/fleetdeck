package health

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ServiceHealth describes the health state of a single container/service.
type ServiceHealth struct {
	Name   string `json:"name"`
	Status string `json:"status"` // human-readable status line (e.g. "Up 2 minutes")
	Health string `json:"health"` // "healthy", "unhealthy", "restarting", "unknown"
}

// HealthReport is the aggregate health state of a project's containers.
type HealthReport struct {
	Healthy  bool            `json:"healthy"`
	Services []ServiceHealth `json:"services"`
	Errors   []string        `json:"errors,omitempty"`
}

// composePSEntry mirrors the JSON fields emitted by docker compose ps --format json.
type composePSEntry struct {
	Name   string `json:"Name"`
	State  string `json:"State"`
	Status string `json:"Status"`
	Health string `json:"Health"`
}

// CheckProject inspects the running containers for the compose project at
// projectPath and returns a health report.
func CheckProject(projectPath string) (*HealthReport, error) {
	cmd := exec.Command("docker", "compose", "ps", "--format", "json")
	cmd.Dir = projectPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return ParseHealthReport(string(out))
}

// ParseHealthReport builds a HealthReport from raw docker compose ps JSON
// output. Exported so it can be used in tests with sample data without
// requiring a running Docker daemon.
func ParseHealthReport(rawJSON string) (*HealthReport, error) {
	report := &HealthReport{Healthy: true}

	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" {
		report.Healthy = false
		report.Errors = append(report.Errors, "no containers found")
		return report, nil
	}

	entries, err := parseComposeJSON(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parsing compose ps output: %w", err)
	}

	if len(entries) == 0 {
		report.Healthy = false
		report.Errors = append(report.Errors, "no containers found")
		return report, nil
	}

	for _, e := range entries {
		health := classifyHealth(e)
		svc := ServiceHealth{
			Name:   e.Name,
			Status: e.Status,
			Health: health,
		}
		report.Services = append(report.Services, svc)

		switch health {
		case "unhealthy", "restarting":
			report.Healthy = false
			report.Errors = append(report.Errors, fmt.Sprintf("%s is %s", e.Name, health))
		case "unknown":
			// If state is not running at all, mark unhealthy.
			if e.State != "running" {
				report.Healthy = false
				report.Errors = append(report.Errors, fmt.Sprintf("%s has state %q", e.Name, e.State))
			}
		}
	}

	return report, nil
}

// parseComposeJSON handles both JSON-array output and newline-delimited JSON
// objects (older compose versions emit one JSON object per line).
func parseComposeJSON(raw string) ([]composePSEntry, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	// Try array first.
	if strings.HasPrefix(trimmed, "[") {
		var entries []composePSEntry
		if err := json.Unmarshal([]byte(trimmed), &entries); err == nil {
			return entries, nil
		}
	}

	// Fall back to newline-delimited JSON objects.
	var entries []composePSEntry
	for _, line := range strings.Split(trimmed, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e composePSEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("invalid JSON line %q: %w", line, err)
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// classifyHealth maps the compose container entry to one of the canonical
// health strings: "healthy", "unhealthy", "restarting", "unknown".
func classifyHealth(e composePSEntry) string {
	// Docker Compose v2 populates the Health field for containers with
	// HEALTHCHECK configured. Use it when available.
	switch strings.ToLower(e.Health) {
	case "healthy":
		return "healthy"
	case "unhealthy":
		return "unhealthy"
	}

	state := strings.ToLower(e.State)
	if state == "restarting" {
		return "restarting"
	}
	if state == "running" {
		return "healthy"
	}

	return "unknown"
}

// WaitForHealthy polls CheckProject every 2 seconds up to timeout, returning
// the last report obtained. It returns as soon as the project is healthy or
// the timeout elapses.
func WaitForHealthy(projectPath string, timeout time.Duration) *HealthReport {
	deadline := time.Now().Add(timeout)
	var report *HealthReport

	for time.Now().Before(deadline) {
		r, err := CheckProject(projectPath)
		if err == nil {
			report = r
			if r.Healthy {
				return r
			}
		}
		time.Sleep(2 * time.Second)
	}

	// One final check.
	if r, err := CheckProject(projectPath); err == nil {
		return r
	}
	return report
}
