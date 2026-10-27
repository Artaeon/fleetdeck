package securitywatch

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fleetdeck/fleetdeck/internal/monitor"
)

type delivery struct {
	Digest  string `json:"digest"`
	Alerted bool   `json:"alerted"`
}
type State struct {
	Version    int                 `json:"version"`
	Report     Report              `json:"report"`
	Deliveries map[string]delivery `json:"deliveries"`
}

func Load(path string) (State, error) {
	state := State{Version: 1, Deliveries: map[string]delivery{}}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 4<<20 {
		return state, fmt.Errorf("security state must be a private regular file under 4 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if json.Unmarshal(data, &state) != nil || state.Version != 1 {
		return state, fmt.Errorf("invalid security watcher state")
	}
	if state.Deliveries == nil {
		state.Deliveries = map[string]delivery{}
	}
	return state, nil
}
func Save(path string, state State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 4<<20 {
		return fmt.Errorf("security state exceeds 4 MiB")
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".security-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

// Notify advances each provider's digest only on successful delivery. Failed
// providers retry at the next scheduled run without repeating successful ones.
func Notify(state *State, report Report, label string, providers []monitor.AlertProvider) error {
	if state.Deliveries == nil {
		state.Deliveries = map[string]delivery{}
	}
	canonical := report
	canonical.CheckedAt = time.Time{}
	data, err := json.Marshal(canonical)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	critical, warning, unknown := report.Counts()
	active := critical+warning+unknown > 0
	level := "info"
	if critical > 0 {
		level = "critical"
	} else if warning+unknown > 0 {
		level = "warning"
	}
	failures := 0
	for _, provider := range providers {
		previous := state.Deliveries[provider.Name()]
		if previous.Digest == digest {
			continue
		}
		if active || previous.Alerted {
			title := "Security checks need attention"
			if !active {
				title = "Previously reported security checks are clear"
			}
			message := fmt.Sprintf("%d critical findings, %d warnings or updates, %d incomplete checks. Review the private FleetDeck security report. No software was updated automatically.", critical, warning, unknown)
			if report.Host.RebootRequired {
				message += " The host reports a required restart; schedule it with verified recovery access."
			}
			if provider.Send(monitor.Alert{Level: level, Title: title, Message: message, Target: label, Timestamp: report.CheckedAt}) != nil {
				failures++
				continue
			}
		}
		state.Deliveries[provider.Name()] = delivery{Digest: digest, Alerted: active}
	}
	state.Report = report
	if failures > 0 {
		return fmt.Errorf("security notification delivery failed for %d channel(s); pending deliveries will retry", failures)
	}
	return nil
}
