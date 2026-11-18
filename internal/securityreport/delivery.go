package securityreport

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/deliverystate"
	"github.com/fleetdeck/fleetdeck/internal/deploy"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

type pendingReport struct {
	QueuedAt    time.Time     `json:"queued_at"`
	Destination string        `json:"destination"`
	Alert       monitor.Alert `json:"alert"`
}
type deliveryState struct {
	Version int                      `json:"version"`
	Labels  string                   `json:"labels"`
	Sent    map[string]time.Time     `json:"sent"`
	Pending map[string]pendingReport `json:"pending"`
}

const reportStateLimit = 1 << 20

func readState(path string) (deliveryState, error) {
	var s deliveryState
	if err := deliverystate.Read(path, &s, reportStateLimit); err != nil {
		return s, err
	}
	if (s.Version != 1 && s.Version != 2) || s.Labels == "" || s.Sent == nil {
		return s, errors.New("invalid report delivery state")
	}
	if s.Version == 1 {
		if s.Pending != nil {
			return s, errors.New("unexpected legacy pending state")
		}
		return s, nil
	}
	if s.Pending == nil || len(s.Pending) > 32 {
		return s, errors.New("invalid pending report set")
	}
	if len(s.Pending) > 0 {
		if err := s.destinations().Validate(); err != nil {
			return s, err
		}
	}
	for _, p := range s.Pending {
		if p.QueuedAt.IsZero() || p.Alert.Timestamp.IsZero() || p.Alert.Title != "FleetDeck security report" || p.Alert.Target != "fleet" || p.Alert.Message == "" {
			return s, errors.New("invalid pending report snapshot")
		}
	}
	return s, nil
}

func (s deliveryState) destinations() monitor.Destinations {
	d := monitor.Destinations{}
	for name, p := range s.Pending {
		d[name] = p.Destination
	}
	return d
}

// Deliver freezes each due channel's report before sending. Independent intervals
// continue for working channels even while another channel retains an old report.
func Deliver(path string, r Report, language string, interval time.Duration, providers []monitor.AlertProvider) error {
	if path == "" || len(providers) == 0 || interval < 24*time.Hour || interval > 30*24*time.Hour || r.GeneratedAt.IsZero() || len(r.Nodes) == 0 {
		return errors.New("delivery requires a dated report, private state, channels and a 1–30 day interval")
	}
	current, err := monitor.BindProviders(providers)
	if err != nil {
		return err
	}
	if err := deliverystate.PrivateDir(path); err != nil {
		return err
	}
	lock, err := deploy.AcquireLock(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer lock.Release()
	s, err := readState(path)
	if os.IsNotExist(err) {
		s = deliveryState{Sent: map[string]time.Time{}}
	} else if err != nil {
		return err
	}
	labels := make([]string, 0, len(r.Nodes))
	for _, n := range r.Nodes {
		labels = append(labels, n.Label)
	}
	identity := strings.Join(labels, ",")
	if s.Labels != "" && s.Labels != identity {
		return errors.New("input set changed: review enrollment and use a separate delivery state")
	}
	s.Labels = identity
	for _, at := range s.Sent {
		if at.After(r.GeneratedAt.Add(30 * time.Second)) {
			return errors.New("delivery clock moved backwards")
		}
	}
	if len(s.Pending) > 0 {
		if err := s.destinations().Check(current); err != nil {
			return err
		}
		for _, p := range s.Pending {
			if p.Alert.Timestamp.After(r.GeneratedAt.Add(30 * time.Second)) {
				return errors.New("delivery clock moved backwards")
			}
		}
	}
	// v1 recorded successful sends only. Its interval checkpoints still apply.
	dirty := s.Version != 2
	s.Version = 2
	if s.Pending == nil {
		s.Pending = map[string]pendingReport{}
	}
	hadBacklog := len(s.Pending) > 0
	queuedAt := time.Now().UTC()
	for name, id := range current {
		if _, ok := s.Pending[name]; ok {
			continue
		}
		if _, known := s.Sent[name]; hadBacklog && !known {
			continue
		}
		if last := s.Sent[name]; !last.IsZero() && r.GeneratedAt.Sub(last) < interval {
			continue
		}
		alert := monitor.Alert{Level: r.Level(), Title: "FleetDeck security report", Message: r.Text(language, true), Target: "fleet", Timestamp: r.GeneratedAt}
		s.Pending[name] = pendingReport{QueuedAt: queuedAt, Destination: id, Alert: alert}
		dirty = true
	}
	if len(s.Pending) > 32 {
		return errors.New("too many outstanding report channels")
	}
	if dirty {
		if err := deliverystate.Write(path, s, reportStateLimit); err != nil {
			return err
		}
	}
	for _, p := range providers {
		item, ok := s.Pending[p.Name()]
		if !ok {
			continue
		}
		if err := (monitor.Destinations{p.Name(): item.Destination}).Verify(p); err != nil {
			return err
		}
		if err := p.Send(item.Alert); err != nil {
			continue
		}
		s.Sent[p.Name()] = r.GeneratedAt
		delete(s.Pending, p.Name())
		if err := deliverystate.Write(path, s, reportStateLimit); err != nil {
			return err
		}
	}
	if len(s.Pending) > 0 {
		return fmt.Errorf("report delivery pending for %d channels; successful channels are checkpointed", len(s.Pending))
	}
	return nil
}

func ReadBacklog(path string) (monitor.Backlog, error) {
	b := monitor.Backlog{Coverage: "tracked", Items: []monitor.Waiting{}}
	s, err := readState(path)
	if err != nil {
		return b, err
	}
	if s.Version == 1 {
		b.Coverage = "legacy_successes_only"
		return b, nil
	}
	for name, p := range s.Pending {
		b.Items = append(b.Items, monitor.WaitingFor(p.Alert, "security_report", name, p.QueuedAt))
	}
	b.Sort()
	return b, nil
}
