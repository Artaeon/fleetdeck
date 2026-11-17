package securityreport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/deploy"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

type deliveryState struct {
	Version int                  `json:"version"`
	Labels  string               `json:"labels"`
	Sent    map[string]time.Time `json:"sent"`
}

// Deliver checkpoints each provider separately. This file must never be the
// scanner state. A crash after provider acceptance can still duplicate delivery.
func Deliver(path string, r Report, language string, interval time.Duration, providers []monitor.AlertProvider) error {
	if path == "" || len(providers) == 0 || interval < 24*time.Hour || interval > 30*24*time.Hour {
		return fmt.Errorf("delivery requires a private state, channels and a 1–30 day interval")
	}
	dir := filepath.Dir(path)
	i, err := os.Lstat(dir)
	if err != nil || !i.IsDir() || i.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("delivery state parent must be an existing private directory")
	}
	lock, err := deploy.AcquireLock(dir)
	if err != nil {
		return err
	}
	defer lock.Release()
	s := deliveryState{Version: 1, Sent: map[string]time.Time{}}
	i, err = os.Lstat(path)
	if err == nil {
		if !i.Mode().IsRegular() || i.Mode().Perm()&0077 != 0 || i.Size() > 65536 {
			return fmt.Errorf("invalid delivery state permissions or size")
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return fmt.Errorf("cannot read delivery state")
		}
		if json.Unmarshal(b, &s) != nil || s.Version != 1 || s.Sent == nil || s.Labels == "" {
			return fmt.Errorf("invalid delivery state")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("cannot inspect delivery state")
	}
	labels := []string{}
	for _, n := range r.Nodes {
		labels = append(labels, n.Label)
	}
	identity := strings.Join(labels, ",")
	if s.Labels != "" && s.Labels != identity {
		return fmt.Errorf("input set changed: review enrollment and use a separate delivery state")
	}
	s.Labels = identity
	for _, at := range s.Sent {
		if at.After(r.GeneratedAt.Add(30 * time.Second)) {
			return fmt.Errorf("delivery clock moved backwards")
		}
	}
	failed := 0
	for _, p := range providers {
		last := s.Sent[p.Name()]
		if !last.IsZero() && r.GeneratedAt.Sub(last) < interval {
			continue
		}
		if p.Send(monitor.Alert{Level: r.Level(), Title: "FleetDeck security report", Message: r.Text(language, true), Target: "fleet", Timestamp: r.GeneratedAt}) != nil {
			failed++
			continue
		}
		s.Sent[p.Name()] = r.GeneratedAt
		if err := writeState(path, s); err != nil {
			return err
		}
	}
	if failed > 0 {
		return fmt.Errorf("report delivery failed for %d channels; successful channels are checkpointed", failed)
	}
	return nil
}

func writeState(path string, s deliveryState) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".report-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	e := f.Close()
	if err != nil {
		return err
	}
	if e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
