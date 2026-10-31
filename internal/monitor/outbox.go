package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/sys/unix"
)

const maxQueuedAlerts = 1024
const maxAlertStateBytes = 4 << 20

var channelName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type alertDelivery struct {
	Delivered bool      `json:"delivered"`
	Attempts  int       `json:"attempts"`
	NextTry   time.Time `json:"next_try,omitempty"`
	LastError string    `json:"last_error,omitempty"`
}

type queuedAlert struct {
	Alert    Alert                    `json:"alert"`
	Channels map[string]alertDelivery `json:"channels"`
}

type alertDiskState struct {
	Version int                     `json:"version"`
	States  map[string]*targetState `json:"states"`
	Pending []queuedAlert           `json:"pending"`
}

type alertOutbox struct {
	path  string
	lock  *os.File
	state alertDiskState
}

// NewDurableAlertManager persists incident transitions and the resulting alerts
// together, before contacting any provider. One process owns a queue at a time.
func NewDurableAlertManager(providers []AlertProvider, threshold int, path string) (*AlertManager, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("alert state path must be absolute")
	}
	seen := map[string]bool{}
	for _, p := range providers {
		if !channelName.MatchString(p.Name()) || seen[p.Name()] {
			return nil, errors.New("alert providers need distinct bounded channel names")
		}
		seen[p.Name()] = true
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	st, err := os.Lstat(dir)
	if err != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("alert state directory must be private (0700), not a symlink")
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	st, err = lock.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		lock.Close()
		return nil, errors.New("alert lock must be a private regular file")
	}
	if unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB) != nil {
		lock.Close()
		return nil, errors.New("another monitor owns this alert queue")
	}
	state, err := loadAlertState(path)
	if os.IsNotExist(err) {
		state, err = alertDiskState{Version: 1, States: map[string]*targetState{}}, nil
	}
	if err != nil {
		lock.Close()
		return nil, err
	}
	am := NewAlertManager(providers, threshold)
	am.states = state.States
	am.outbox = &alertOutbox{path: path, lock: lock, state: state}
	if err := am.saveOutbox(); err != nil {
		am.Close()
		return nil, err
	}
	return am, nil
}

func loadAlertState(path string) (alertDiskState, error) {
	var state alertDiskState
	f, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return state, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > maxAlertStateBytes {
		return state, errors.New("alert state must be a private regular file within size limits")
	}
	data, err := io.ReadAll(io.LimitReader(f, maxAlertStateBytes+1))
	if err != nil || len(data) > maxAlertStateBytes || json.Unmarshal(data, &state) != nil || state.Version != 1 || state.States == nil {
		return state, errors.New("invalid alert state; refusing to discard pending notifications")
	}
	if len(state.States) > 4096 || len(state.Pending) > maxQueuedAlerts {
		return state, errors.New("alert state exceeds entry limits")
	}
	for _, s := range state.States {
		if s == nil || s.ConsecutiveFailures < 0 || s.ConsecutiveStatus < 0 {
			return state, errors.New("invalid persisted incident state")
		}
	}
	for _, a := range state.Pending {
		if len(a.Channels) == 0 || len(a.Channels) > 32 || a.Alert.Target == "" {
			return state, errors.New("invalid pending alert")
		}
		for name, delivery := range a.Channels {
			if !channelName.MatchString(name) || delivery.Attempts < 0 || delivery.Attempts > 32 || (delivery.LastError != "" && delivery.LastError != "delivery_failed") {
				return state, errors.New("invalid persisted delivery state")
			}
		}
	}
	return state, nil
}

func cloneAlertStates(states map[string]*targetState) map[string]*targetState {
	copy := make(map[string]*targetState, len(states))
	for name, state := range states {
		value := *state
		copy[name] = &value
	}
	return copy
}

func (am *AlertManager) saveOutbox() error {
	am.outbox.state.States = am.states
	if len(am.states) > 4096 || len(am.outbox.state.Pending) > maxQueuedAlerts {
		return errors.New("alert queue full; restore delivery before restarting monitoring")
	}
	data, err := json.Marshal(am.outbox.state)
	if err != nil || len(data) > maxAlertStateBytes {
		return errors.New("alert state exceeds size limit")
	}
	dir := filepath.Dir(am.outbox.path)
	f, err := os.CreateTemp(dir, ".alert-state-*")
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
	if err := os.Rename(f.Name(), am.outbox.path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// RetryPending is called by a single delivery worker. A slow channel does not
// hold the incident lock. Each channel preserves event order independently.
// Acceptance followed by a crash before checkpointing can cause a duplicate:
// transport delivery is at-least-once, never an exactly-once promise.
func (am *AlertManager) RetryPending(now time.Time) error {
	if am.outbox == nil {
		return nil
	}
	for _, provider := range am.providers {
		am.mu.Lock()
		index := -1
		var alert Alert
		for i, item := range am.outbox.state.Pending {
			status, exists := item.Channels[provider.Name()]
			if exists && !status.Delivered {
				if !now.Before(status.NextTry) {
					index, alert = i, item.Alert
				}
				break
			}
		}
		am.mu.Unlock()
		if index < 0 {
			continue
		}
		sendErr := provider.Send(alert)
		am.mu.Lock()
		status := am.outbox.state.Pending[index].Channels[provider.Name()]
		if status.Attempts < 32 {
			status.Attempts++
		}
		if sendErr == nil {
			status.Delivered, status.LastError, status.NextTry = true, "", time.Time{}
		} else {
			// Provider errors can contain tokens, endpoints or recipient addresses.
			status.LastError = "delivery_failed"
			delay := 30 * time.Second * time.Duration(1<<min(status.Attempts-1, 5))
			status.NextTry = now.Add(min(delay, 15*time.Minute))
		}
		am.outbox.state.Pending[index].Channels[provider.Name()] = status
		err := am.saveOutbox()
		am.mu.Unlock()
		if err != nil {
			return fmt.Errorf("cannot checkpoint notification delivery: %w", err)
		}
	}
	am.mu.Lock()
	defer am.mu.Unlock()
	kept := make([]queuedAlert, 0, len(am.outbox.state.Pending))
	for _, a := range am.outbox.state.Pending {
		complete := true
		for _, status := range a.Channels {
			complete = complete && status.Delivered
		}
		if !complete {
			kept = append(kept, a)
		}
	}
	if len(kept) != len(am.outbox.state.Pending) {
		am.outbox.state.Pending = kept
		return am.saveOutbox()
	}
	return nil
}

func (am *AlertManager) Close() error {
	if am.outbox != nil && am.outbox.lock != nil {
		err := am.outbox.lock.Close()
		am.outbox.lock = nil
		return err
	}
	return nil
}

type DeliveryStatus struct {
	Pending  int                        `json:"pending_alerts"`
	Channels map[string]ChannelDelivery `json:"channels"`
}
type ChannelDelivery struct {
	Pending int       `json:"pending"`
	Failed  int       `json:"failed"`
	NextTry time.Time `json:"next_try,omitempty"`
}

// ReadDeliveryStatus exposes only counters and retry times, never alert content
// or credentials. A removed provider remains pending until restored by an operator.
func ReadDeliveryStatus(path string) (DeliveryStatus, error) {
	s, err := loadAlertState(path)
	if err != nil {
		return DeliveryStatus{}, err
	}
	out := DeliveryStatus{Pending: len(s.Pending), Channels: map[string]ChannelDelivery{}}
	for _, item := range s.Pending {
		for name, state := range item.Channels {
			if state.Delivered {
				continue
			}
			c := out.Channels[name]
			c.Pending++
			if state.LastError != "" {
				c.Failed++
			}
			if c.Pending == 1 {
				c.NextTry = state.NextTry
			}
			out.Channels[name] = c
		}
	}
	return out, nil
}
