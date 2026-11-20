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
	QueuedAt        time.Time         `json:"queued_at"`
	Destination     string            `json:"destination"`
	Alert           monitor.Alert     `json:"alert"`
	Period          string            `json:"period,omitempty"`
	Evidence        map[string]string `json:"evidence,omitempty"`
	PDF             []byte            `json:"pdf,omitempty"`
	ArchiveRequired bool              `json:"archive_required,omitempty"`
	PDFHash         string            `json:"pdf_sha256,omitempty"`
}
type deliveryState struct {
	Version     int                           `json:"version"`
	EvaluatedAt time.Time                     `json:"evaluated_at,omitzero"`
	Edition     string                        `json:"edition,omitempty"`
	Schedule    string                        `json:"schedule,omitempty"`
	Checkpoints map[string]briefingCheckpoint `json:"checkpoints,omitempty"`
	Labels      string                        `json:"labels"`
	Sent        map[string]time.Time          `json:"sent"`
	Pending     map[string]pendingReport      `json:"pending"`
}

const reportStateLimit = 1 << 20

func readState(path string) (deliveryState, error) {
	var s deliveryState
	if err := deliverystate.Read(path, &s, reportStateLimit); err != nil {
		return s, err
	}
	if (s.Version != 1 && s.Version != 2 && s.Version != 3) || s.Labels == "" || s.Sent == nil {
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
	if err := s.validateBriefings(); err != nil {
		return s, err
	}
	for _, p := range s.Pending {
		if p.QueuedAt.IsZero() || p.Alert.Timestamp.IsZero() || p.Alert.Target != "fleet" || p.Alert.Message == "" || !s.validTitle(p.Alert.Title) {
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
	return deliver(path, r, language, interval, providers, nil)
}

// DeliverBriefing uses a separate calendar checkpoint and the same recipient-bound queue.
func DeliverBriefing(path string, r Report, language string, s Schedule, providers []monitor.AlertProvider) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if language != "de" && language != "en" {
		return errors.New("unsupported briefing language")
	}
	return deliver(path, r, language, 24*time.Hour, providers, &s)
}

func deliver(path string, r Report, language string, interval time.Duration, providers []monitor.AlertProvider, schedule *Schedule) error {
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
	version := 2
	dirty := false
	if schedule != nil {
		version = 3
		if s.EvaluatedAt.After(r.GeneratedAt.Add(30 * time.Second)) {
			return errors.New("evaluation clock moved backwards")
		}
		if !s.EvaluatedAt.Equal(r.GeneratedAt) {
			s.EvaluatedAt = r.GeneratedAt.UTC()
			dirty = true
		}
		if err := s.bindSchedule(*schedule); err != nil {
			return err
		}
	} else if s.Version == 3 {
		return errors.New("calendar reports require their own delivery state")
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
	dirty = dirty || s.Version != version
	s.Version = version
	if s.Pending == nil {
		s.Pending = map[string]pendingReport{}
	}
	hadBacklog := len(s.Pending) > 0
	queuedAt := time.Now().UTC()
	for name, id := range current {
		if _, ok := s.Pending[name]; ok {
			continue
		}
		_, sent := s.Sent[name]
		_, baseline := s.Checkpoints[name]
		if hadBacklog && !sent && !baseline {
			continue
		}
		if schedule != nil {
			item, baseline, err := s.nextBriefing(name, r, language, *schedule)
			if err != nil {
				return err
			}
			if baseline != nil {
				s.Checkpoints[name] = *baseline
				dirty = true
			}
			if item != nil {
				item.QueuedAt = queuedAt
				item.Destination = id
				s.Pending[name] = *item
				dirty = true
			}
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
		var sendErr error
		if len(item.PDF) > 0 {
			if item.ArchiveRequired {
				if schedule == nil || schedule.ArchiveDir == "" {
					return errors.New("pending PDF requires its private archive")
				}
				if err := archivePDF(schedule.ArchiveDir, item); err != nil {
					return err
				}
			}
			sender, ok := p.(interface {
				SendReportPDF(monitor.Alert, []byte) error
			})
			if !ok {
				return errors.New("pending report requires a PDF-capable email provider")
			}
			sendErr = sender.SendReportPDF(item.Alert, item.PDF)
		} else {
			sendErr = p.Send(item.Alert)
		}
		if sendErr != nil {
			continue
		}
		if schedule != nil {
			s.Checkpoints[p.Name()] = briefingCheckpoint{Period: item.Period, Evidence: item.Evidence}
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
