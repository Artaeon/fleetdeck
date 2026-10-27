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

	"github.com/fleetdeck/fleetdeck/internal/monitor"
	"golang.org/x/sys/unix"
)

type pendingBatch struct {
	Cursor    string          `json:"cursor"`
	Alert     monitor.Alert   `json:"alert"`
	Delivered map[string]bool `json:"delivered"`
}
type relayState struct {
	Since   string        `json:"since"`
	Cursor  string        `json:"cursor"`
	Pending *pendingBatch `json:"pending,omitempty"`
}

func saveState(path string, state *relayState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".alerts-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func deliver(path string, state *relayState, providers []monitor.AlertProvider) error {
	if state.Pending == nil {
		return nil
	}
	failed := false
	for _, provider := range providers {
		if state.Pending.Delivered[provider.Name()] {
			continue
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
	if failed {
		return errors.New("notification delivery failed; pending batch retained for retry")
	}
	state.Cursor = state.Pending.Cursor
	state.Pending = nil
	return saveState(path, state)
}

func readJournal(ctx context.Context, state *relayState) ([]JournalEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	args := []string{"--no-pager", "--no-tail", "--output=json", "--output-fields=__CURSOR,_UID,SYSLOG_IDENTIFIER,MESSAGE", "_UID=0", "SYSLOG_IDENTIFIER=sshd", "SYSLOG_IDENTIFIER=sshd-session", "SYSLOG_IDENTIFIER=fleetdeck-audit"}
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
	info, err := os.Stat(filepath.Dir(path))
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("relay state directory must already exist with mode 0700")
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		return errors.New("another alert relay owns this state file")
	}
	state := relayState{Since: time.Now().UTC().Format("2006-01-02 15:04:05 UTC")}
	if st, err := os.Lstat(path); err == nil {
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 65536 {
			return errors.New("relay state must be a private regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if json.Unmarshal(data, &state) != nil || state.Since == "" || (state.Pending != nil && state.Pending.Delivered == nil) {
			return errors.New("invalid relay state; refusing to discard cursor")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := saveState(path, &state); err != nil {
		return err
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
				state.Pending = &pendingBatch{Cursor: cursor, Alert: *alert, Delivered: map[string]bool{}}
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
