package securityreport

import (
	"fmt"
	"time"
)

// BriefingStatus can be exported to a separately operated monitor. It contains
// timestamps and a backlog count, never the report, enrollment or addresses.
type BriefingStatus struct {
	Edition        string               `json:"edition"`
	LastEvaluation time.Time            `json:"last_evaluation"`
	LastHandoff    map[string]time.Time `json:"last_handoff"`
	Pending        int                  `json:"pending"`
	OldestPending  *time.Time           `json:"oldest_pending,omitempty"`
	Status         string               `json:"status"`
}

func Status(path string, now time.Time, maxAge time.Duration) (BriefingStatus, error) {
	var result BriefingStatus
	if maxAge < time.Minute || maxAge > 7*24*time.Hour {
		return result, fmt.Errorf("invalid evaluation age")
	}
	s, err := readState(path)
	if err != nil {
		return result, err
	}
	if s.Version != 3 {
		return result, fmt.Errorf("this state has no calendar evaluation history")
	}
	result = BriefingStatus{Edition: s.Edition, LastEvaluation: s.EvaluatedAt, LastHandoff: s.Sent, Pending: len(s.Pending), Status: "current"}
	for _, p := range s.Pending {
		at := p.QueuedAt.UTC()
		if result.OldestPending == nil || at.Before(*result.OldestPending) {
			result.OldestPending = &at
		}
	}
	if len(s.Pending) > 0 {
		result.Status = "pending"
	}
	if s.EvaluatedAt.IsZero() || s.EvaluatedAt.After(now.Add(30*time.Second)) || now.Sub(s.EvaluatedAt) > maxAge {
		result.Status = "stale"
	}
	return result, nil
}
