package monitor

import (
	"fmt"
	"sort"
	"time"
)

// CheckSummary deliberately excludes URLs, raw transport errors and headers.
// It describes an observation, not the availability of the entire server.
type CheckSummary struct {
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	Detail         string     `json:"detail"`
	StatusCode     int        `json:"status_code"`
	ResponseTimeMS int64      `json:"response_time_ms"`
	CheckedAt      *time.Time `json:"checked_at,omitempty"`
}

func Summarize(state *MonitorState, now time.Time) ([]CheckSummary, error) {
	if len(state.Targets) > 100 {
		return nil, fmt.Errorf("too many monitoring targets")
	}
	summaries := make([]CheckSummary, 0, len(state.Targets))
	seen := make(map[string]bool)
	for _, target := range state.Targets {
		if target.Name == "" || len(target.Name) > 100 || seen[target.Name] {
			return nil, fmt.Errorf("invalid monitoring target names")
		}
		seen[target.Name] = true
		item := CheckSummary{Name: target.Name, Status: "unknown", Detail: "No completed check yet."}
		result, exists := state.Results[target.Name]
		if !exists || result.CheckedAt.IsZero() {
			summaries = append(summaries, item)
			continue
		}
		item.CheckedAt = &result.CheckedAt
		item.StatusCode = result.StatusCode
		item.ResponseTimeMS = result.ResponseTime.Milliseconds()
		interval, timeout := target.Interval, target.Timeout
		if interval <= 0 {
			interval = 30 * time.Second
		}
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		// Bound arithmetic when reading an operator-owned but potentially corrupt file.
		if interval > 24*time.Hour || timeout > time.Hour {
			return nil, fmt.Errorf("invalid monitoring timing")
		}
		staleAfter := 3*interval + timeout
		if staleAfter < time.Minute {
			staleAfter = time.Minute
		}
		switch {
		case result.CheckedAt.After(now.Add(time.Minute)):
			item.Detail = "Check timestamp is in the future; verify the monitor clock."
		case now.Sub(result.CheckedAt) > staleAfter:
			item.Status, item.Detail = "stale", "No recent check. Current application health is unknown."
		default:
			item.Status = result.HealthStatus()
			switch item.Status {
			case "blocked":
				item.Detail = "Cloudflare browser challenge; application health is unknown."
			case "healthy":
				item.Detail = "Expected HTTP response received."
			default:
				item.Detail = "Unexpected HTTP response."
				if result.Error != "" || result.StatusCode == 0 {
					item.Detail = "Request failed. Inspect the private monitor state for details."
				}
			}
		}
		summaries = append(summaries, item)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}
