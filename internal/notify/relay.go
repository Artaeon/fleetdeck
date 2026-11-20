package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/deliverystate"
	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"golang.org/x/sys/unix"
)

type pendingBatch struct {
	Cursor       string               `json:"cursor"`
	Alert        monitor.Alert        `json:"alert"`
	Delivered    map[string]bool      `json:"acknowledged,omitempty"`
	Destinations monitor.Destinations `json:"destinations"`
}
type relayState struct {
	Version int           `json:"version,omitempty"`
	Since   string        `json:"since"`
	Cursor  string        `json:"cursor"`
	Pending *pendingBatch `json:"pending,omitempty"`
}

func saveState(path string, state *relayState) error {
	return deliverystate.Write(path, state, 65536)
}

func readState(path string) (relayState, error) {
	var s relayState
	if err := deliverystate.Read(path, &s, 65536); err != nil {
		return s, err
	}
	if s.Since == "" || (s.Version != 0 && s.Version != 2) {
		return s, errors.New("invalid relay state")
	}
	if p := s.Pending; p != nil {
		if p.Cursor == "" {
			return s, errors.New("missing pending journal cursor")
		}
		if s.Version == 2 {
			if p.Alert.Timestamp.IsZero() {
				return s, errors.New("missing host activity timestamp")
			}
			if err := p.Destinations.Validate(); err != nil {
				return s, err
			}
			for name, done := range p.Delivered {
				if p.Destinations[name] == "" || !done {
					return s, errors.New("invalid host activity acknowledgement")
				}
			}
		}
	}
	return s, nil
}

func deliver(path string, state *relayState, providers []monitor.AlertProvider) error {
	p := state.Pending
	if p == nil {
		return nil
	}
	if state.Version != 2 {
		return errors.New("unbound legacy host activity; drain with the previous verified configuration and version")
	}
	current, err := monitor.BindProviders(providers)
	if err != nil {
		return err
	}
	for name := range p.Delivered {
		delete(current, name)
	}
	if err := p.Destinations.Check(current); err != nil {
		return err
	}
	if p.Delivered == nil {
		p.Delivered = map[string]bool{}
	}
	failed := false
	for _, provider := range providers {
		if p.Destinations[provider.Name()] == "" || p.Delivered[provider.Name()] {
			continue
		}
		if err := p.Destinations.Verify(provider); err != nil {
			return err
		}
		if provider.Send(state.Pending.Alert) != nil {
			failed = true
			continue
		}
		state.Pending.Delivered[provider.Name()] = true
		if err := saveState(path, state); err != nil {
			return err
		}
	}
	if failed || len(p.Delivered) != len(p.Destinations) {
		return errors.New("notification delivery failed; pending batch retained for retry")
	}
	state.Cursor = state.Pending.Cursor
	state.Pending = nil
	return saveState(path, state)
}

func readJournal(ctx context.Context, state *relayState) ([]JournalEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{"--no-pager", "--no-tail", "--output=json", "--output-fields=__CURSOR,__REALTIME_TIMESTAMP,_UID,SYSLOG_IDENTIFIER,MESSAGE", "_UID=0", "SYSLOG_IDENTIFIER=sshd", "SYSLOG_IDENTIFIER=sshd-session", "SYSLOG_IDENTIFIER=fleetdeck-audit"}
	if state.Cursor != "" {
		args = append(args, "--after-cursor="+state.Cursor)
	} else {
		args = append(args, "--since="+state.Since)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stderr = io.Discard
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot read the local system journal")
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 128*1024)
	var entries []JournalEntry
	for scanner.Scan() {
		var entry JournalEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Cursor == "" {
			cancel()
			_ = cmd.Wait()
			return nil, fmt.Errorf("unexpected journal entry; cursor left unchanged")
		}
		entries = append(entries, entry)
		if len(entries) == 1000 {
			cancel()
			_ = cmd.Wait()
			return entries, nil
		}
	}
	if err = scanner.Err(); err != nil {
		cancel()
		_ = cmd.Wait()
		return nil, fmt.Errorf("journal entry exceeds safe bounds")
	}
	if err = cmd.Wait(); err != nil {
		return nil, fmt.Errorf("journal read failed; check permissions and cursor retention")
	}
	return entries, nil
}

// Watch persists a pending batch before sending, and per-channel successes after
// sending. A process crash in between may duplicate a message (at-least-once).
// Host authentication never waits for or depends on this notification service.
func Watch(ctx context.Context, path, label string, interval time.Duration, providers []monitor.AlertProvider) error {
	if !filepath.IsAbs(path) || len(providers) == 0 {
		return errors.New("absolute private state path and notification providers required")
	}
	if err := deliverystate.PrivateDir(path); err != nil {
		return err
	}
	destinations, err := monitor.BindProviders(providers)
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return errors.New("another alert relay owns this state file")
	}
	state, err := readState(path)
	if os.IsNotExist(err) {
		state = relayState{Version: 2, Since: time.Now().UTC().Format("2006-01-02 15:04:05 UTC")}
		if err := saveState(path, &state); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if state.Pending == nil {
		state.Version = 2
	}
	for {
		if err := deliver(path, &state, providers); err != nil {
			return err
		}
		entries, err := readJournal(ctx, &state)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if len(entries) > 0 {
			cursor := entries[len(entries)-1].Cursor
			if alert := Summarize(entries, label); alert != nil {
				state.Pending = &pendingBatch{Cursor: cursor, Alert: *alert, Destinations: destinations}
			} else {
				state.Cursor = cursor
			}
			if err := saveState(path, &state); err != nil {
				return err
			}
			if err := deliver(path, &state, providers); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
		}
	}
}

func ReadBacklog(path string) (monitor.Backlog, error) {
	b := monitor.Backlog{Coverage: "tracked", Items: []monitor.Waiting{}}
	s, err := readState(path)
	if err != nil || s.Pending == nil {
		return b, err
	}
	p := s.Pending
	if s.Version == 0 {
		b.Coverage = "legacy_unknown_recipients"
		b.Items = append(b.Items, monitor.WaitingFor(p.Alert, "host_activity", "", time.Time{}))
		return b, nil
	}
	for name := range p.Destinations {
		if !p.Delivered[name] {
			b.Items = append(b.Items, monitor.WaitingFor(p.Alert, "host_activity", name, p.Alert.Timestamp))
		}
	}
	b.Sort()
	return b, nil
}
