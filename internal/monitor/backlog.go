package monitor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Waiting describes one outstanding channel delivery without publishing content,
// targets, addresses or destination fingerprints.
type Waiting struct {
	Reference    string     `json:"reference"`
	Kind         string     `json:"kind"`
	Channel      string     `json:"channel,omitempty"`
	PendingSince *time.Time `json:"pending_since"`
	ObservedAt   *time.Time `json:"observed_at,omitempty"`
}

func WaitingFor(alert Alert, kind, channel string, since time.Time) Waiting {
	b, _ := json.Marshal(alert)
	sum := sha256.Sum256(b)
	w := Waiting{Reference: fmt.Sprintf("%x", sum[:12]), Kind: kind, Channel: channel}
	if !since.IsZero() {
		at := since.UTC()
		w.PendingSince = &at
	}
	if !alert.Timestamp.IsZero() {
		at := alert.Timestamp.UTC()
		w.ObservedAt = &at
	}
	return w
}

// Backlog distinguishes a verified empty queue from older state that only
// recorded successes or cannot identify all original recipients.
type Backlog struct {
	Coverage string    `json:"coverage"`
	Items    []Waiting `json:"items"`
}

func (b *Backlog) Sort() {
	sort.SliceStable(b.Items, func(i, j int) bool {
		a, c := b.Items[i], b.Items[j]
		if a.PendingSince == nil && c.PendingSince != nil {
			return true
		}
		if a.PendingSince != nil && c.PendingSince == nil {
			return false
		}
		if a.PendingSince != nil && !a.PendingSince.Equal(*c.PendingSince) {
			return a.PendingSince.Before(*c.PendingSince)
		}
		if a.Reference != c.Reference {
			return a.Reference < c.Reference
		}
		return a.Channel < c.Channel
	})
}
