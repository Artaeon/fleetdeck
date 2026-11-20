package securityreport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

type briefingCheckpoint struct {
	Period   string            `json:"period"`
	Evidence map[string]string `json:"evidence,omitempty"`
}

func (s *deliveryState) bindSchedule(cfg Schedule) error {
	key := fmt.Sprintf("%s/%d/%d/%s", cfg.Location, cfg.Hour, cfg.Weekday, cfg.FirstDate)
	if s.Version != 0 && (s.Version != 3 || s.Edition != cfg.Edition || s.Schedule != key) {
		return fmt.Errorf("changed report edition or schedule requires a separate delivery state")
	}
	s.Edition = cfg.Edition
	s.Schedule = key
	if s.Checkpoints == nil {
		s.Checkpoints = map[string]briefingCheckpoint{}
	}
	return nil
}

func briefingTitle(edition string, de bool) string {
	if edition == "weekly" {
		return words(de, "Weekly security review", "Wöchentlicher Sicherheitsbericht")
	}
	return words(de, "Daily security changes", "Sicherheitsänderungen des Tages")
}

func (s deliveryState) validTitle(title string) bool {
	if s.Version != 3 {
		return title == "FleetDeck security report"
	}
	return title == briefingTitle(s.Edition, true) || title == briefingTitle(s.Edition, false)
}

func (s deliveryState) validCheckpoint(c briefingCheckpoint) bool {
	if _, err := time.Parse(time.DateOnly, c.Period); err != nil {
		return false
	}
	if s.Edition == "weekly" {
		return len(c.Evidence) == 0
	}
	labels := strings.Split(s.Labels, ",")
	if len(c.Evidence) != len(labels) {
		return false
	}
	for _, label := range labels {
		d, err := hex.DecodeString(c.Evidence[label])
		if err != nil || len(d) != sha256.Size {
			return false
		}
	}
	return true
}

func (s deliveryState) validateBriefings() error {
	if s.Version < 3 {
		if s.Edition != "" || s.Schedule != "" || len(s.Checkpoints) > 0 {
			return fmt.Errorf("unexpected calendar metadata in interval state")
		}
	} else {
		if (s.Edition != "weekly" && s.Edition != "daily") || s.Schedule == "" || len(s.Checkpoints) > 32 {
			return fmt.Errorf("invalid report schedule")
		}
		for _, c := range s.Checkpoints {
			if !s.validCheckpoint(c) {
				return fmt.Errorf("invalid report calendar checkpoint")
			}
		}
	}
	for channel, p := range s.Pending {
		if s.Version == 3 && !s.validCheckpoint(briefingCheckpoint{p.Period, p.Evidence}) {
			return fmt.Errorf("invalid queued period or evidence")
		}
		expect := s.Version == 3 && s.Edition == "weekly" && channel == "email"
		if expect {
			if monitor.ValidateReportPDF(p.PDF) != nil || fmt.Sprintf("%x", sha256.Sum256(p.PDF)) != p.PDFHash {
				return fmt.Errorf("missing or damaged queued PDF")
			}
		} else if len(p.PDF) > 0 || p.PDFHash != "" || p.ArchiveRequired {
			return fmt.Errorf("unexpected report attachment")
		}
	}
	return nil
}

func (s deliveryState) nextBriefing(name string, r Report, language string, cfg Schedule) (*pendingReport, *briefingCheckpoint, error) {
	boundary := cfg.boundary(r.GeneratedAt)
	key := boundary.Format(time.DateOnly)
	last := s.Checkpoints[name]
	if last.Period > key {
		return nil, nil, fmt.Errorf("report calendar moved backwards")
	}
	if r.GeneratedAt.Before(boundary) || last.Period == key || key < cfg.FirstDate {
		return nil, nil, nil
	}
	item := pendingReport{Period: key}
	var changes map[string]bool
	if cfg.Edition == "daily" {
		item.Evidence = r.materialState()
		changes = map[string]bool{}
		for _, n := range r.Nodes {
			if last.Evidence == nil && n.Status == "checked" && !n.RebootRequired && n.Critical+n.Warnings+n.Unknown == 0 {
				continue
			}
			if item.Evidence[n.Label] != last.Evidence[n.Label] {
				changes[n.Label] = true
			}
		}
		if len(changes) == 0 {
			return nil, &briefingCheckpoint{key, item.Evidence}, nil
		}
	}
	item.Alert = monitor.Alert{Level: r.Level(), Title: briefingTitle(cfg.Edition, language == "de"), Message: r.briefingText(cfg, language, changes), Target: "fleet", Timestamp: r.GeneratedAt.UTC()}
	if cfg.Edition == "weekly" && name == "email" {
		pdf, err := r.WeeklyPDF(cfg, language)
		if err != nil {
			return nil, nil, err
		}
		if err = monitor.ValidateReportPDF(pdf); err != nil {
			return nil, nil, err
		}
		item.ArchiveRequired = cfg.ArchiveDir != ""
		item.PDF = pdf
		item.PDFHash = fmt.Sprintf("%x", sha256.Sum256(pdf))
	}
	return &item, nil, nil
}
