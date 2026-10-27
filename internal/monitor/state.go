package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// MonitorState is the on-disk representation of monitor state, used for
// persistence across restarts.
type MonitorState struct {
	Targets   []Target               `json:"targets"`
	Results   map[string]CheckResult `json:"results"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// SaveState marshals the current targets and results to JSON and writes them
// atomically to path (write to a temporary file, then rename).
func SaveState(path string, targets []Target, results []CheckResult) error {
	resultMap := make(map[string]CheckResult, len(results))
	for _, r := range results {
		resultMap[r.Target.Name] = r
	}

	state := MonitorState{
		Targets:   targets,
		Results:   resultMap,
		UpdatedAt: time.Now(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling monitor state: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	file, err := os.CreateTemp(dir, ".monitor-state-*")
	if err != nil {
		return fmt.Errorf("creating private state file: %w", err)
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("writing temporary state file: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming state file: %w", err)
	}

	return nil
}

// LoadState reads and unmarshals a previously saved monitor state from path.
func LoadState(path string) (*MonitorState, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("monitor state must be a regular file")
	}
	const maxStateBytes = 2 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxStateBytes+1))
	if err != nil || len(data) > maxStateBytes {
		return nil, fmt.Errorf("monitor state unreadable or larger than 2 MiB")
	}

	var state MonitorState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshaling state file: %w", err)
	}

	return &state, nil
}

// SaveStateToDisk persists the monitor's current state to its configured
// StatePath. Returns nil if no StatePath is set.
func (m *Monitor) SaveStateToDisk() error {
	if m.StatePath == "" {
		return nil
	}
	m.saveMu.Lock()
	defer m.saveMu.Unlock()

	results := m.Status()
	return SaveState(m.StatePath, m.targets, results)
}
