package securityreport

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/deliverystate"
	"github.com/fleetdeck/fleetdeck/internal/deploy"
	"github.com/fleetdeck/fleetdeck/internal/fleet"
	"github.com/fleetdeck/fleetdeck/internal/securitywatch"
)

type observerRecord struct {
	Version     int                         `json:"version"`
	Host        string                      `json:"host"`
	Fetched     time.Time                   `json:"fetched"`
	Build       string                      `json:"build,omitempty"`
	Observation fleet.Security              `json:"observation"`
	History     []securitywatch.Observation `json:"history,omitempty"`
}

var publicVersion = regexp.MustCompile(`^[a-zA-Z0-9._+-]{1,80}$`)

// CaptureObserver reads a snapshot obtained through the operator's authenticated
// transport, binds it to the enrolled node, and stores only security counts.
func CaptureObserver(path, label string, input io.Reader, now time.Time) error {
	if !identifier.MatchString(label) {
		return errors.New("invalid enrolled host")
	}
	if err := deliverystate.PrivateDir(path); err != nil {
		return err
	}
	snap, err := fleet.Decode(input, label, now)
	if err != nil {
		return err
	}
	lock, err := deploy.AcquireLock(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer lock.Release()
	var state observerRecord
	err = deliverystate.Read(path, &state, 128<<10)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && (state.Version != 1 || state.Host != label || len(state.History) > 56 || state.Fetched.After(snap.GeneratedAt)) {
		return errors.New("changed observer identity, history or clock")
	}
	state.Version = 1
	state.Host = label
	state.Fetched = snap.GeneratedAt.UTC()
	state.Build = ""
	if publicVersion.MatchString(snap.Version) {
		state.Build = snap.Version
	}
	s := snap.Security
	if s.CheckedAt != nil && !s.CheckedAt.IsZero() && (s.CheckedAt.After(now.Add(30*time.Second)) || s.CheckedAt.After(snap.GeneratedAt.Add(30*time.Second))) {
		return errors.New("invalid observation time")
	}
	if s.CheckedAt != nil && state.Observation.CheckedAt != nil && s.CheckedAt.Before(*state.Observation.CheckedAt) {
		return errors.New("older observer scan cannot supersede a newer one")
	}
	state.Observation = s
	if s.CheckedAt != nil && !s.CheckedAt.IsZero() && now.Sub(*s.CheckedAt) <= 26*time.Hour {
		// The observer protocol provides aggregate image/runtime counts. Package
		// and restart coverage cannot be inferred and is represented as incomplete.
		h := securitywatch.Observation{CheckedAt: s.CheckedAt.UTC(), Critical: s.Critical, Warnings: s.High + s.RuntimeFindings, Unknown: s.Unknown + 1}
		if s.Status == "unknown" && s.Unknown == 0 {
			h.Unknown++
		}
		if n := len(state.History); n == 0 || h.CheckedAt.After(state.History[n-1].CheckedAt) {
			state.History = append(state.History, h)
			if len(state.History) > 56 {
				state.History = state.History[len(state.History)-56:]
			}
		}
	}
	return deliverystate.Write(path, state, 128<<10)
}

func observerNode(in Input, now time.Time, maxAge time.Duration) Node {
	n := Node{Label: in.Label, Source: "observer", Status: "unknown", Unknown: 1, Actions: []string{"restore_observation"}}
	var state observerRecord
	if err := deliverystate.Read(in.Path, &state, 128<<10); err != nil || state.Version != 1 || state.Host != in.Label || len(state.History) > 56 {
		return n
	}
	n.history = state.History
	s := state.Observation
	if s.CheckedAt == nil {
		return n
	}
	n.CheckedAt = s.CheckedAt.UTC()
	if publicVersion.MatchString(state.Build) {
		n.Version = state.Build
	}
	if state.Fetched.IsZero() || state.Fetched.After(now.Add(30*time.Second)) || now.Sub(state.Fetched) > 2*time.Hour || n.CheckedAt.IsZero() || n.CheckedAt.After(now.Add(30*time.Second)) || now.Sub(n.CheckedAt) > maxAge {
		n.Actions = []string{"refresh_observation"}
		return n
	}
	for _, num := range []int{s.Critical, s.High, s.RuntimeFindings, s.Unknown} {
		if num < 0 || num > 1_000_000 {
			return n
		}
	}
	if s.Status != "ok" && s.Status != "attention" && s.Status != "unknown" {
		return n
	}
	n.Critical = s.Critical
	n.Warnings = s.High + s.RuntimeFindings
	n.Unknown = s.Unknown + 1
	if s.Status == "unknown" && s.Unknown == 0 {
		n.Unknown++
	}
	n.Actions = []string{"review_observer_scope"}
	if s.Status == "attention" || n.Critical+n.Warnings > 0 {
		n.Status = "attention"
		n.Actions = []string{"review_observer_findings", "review_observer_scope"}
	}
	n.evidence = fmt.Sprintf("%s/%d/%d/%d/%d", s.Status, s.Critical, s.High, s.RuntimeFindings, s.Unknown)
	return n
}
